package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/pricing"
	"github.com/srunal/srunal-go/internal/promo"
	"github.com/srunal/srunal-go/internal/store"
)

func nowUTC() time.Time { return time.Now().UTC() }

// QuoteRequest is the POST /api/quote body. The client may send a promo code
// with the percentage it received from /api/promos/check; the server always
// prices with its own table and its own promo lookup.
type QuoteRequest struct {
	Product string `json:"product"`
	Works   int    `json:"works"`
	Units   int    `json:"units"`
	Promo   *struct {
		Code string `json:"code"`
		Pct  int    `json:"pct"`
	} `json:"promo"`
}

// Quote computes the price breakdown for the order form (debounced 300 ms by
// the client, so it stays a single indexed lookup plus arithmetic).
func (s *Service) Quote(ctx context.Context, request QuoteRequest) (domain.Quote, error) {
	config, err := s.Store.GetPricing(ctx)
	if err != nil {
		return domain.Quote{}, err
	}

	var applied *pricing.AppliedPromo
	if request.Promo != nil && strings.TrimSpace(request.Promo.Code) != "" {
		code, err := s.Store.GetPromoByCode(ctx, request.Promo.Code)
		switch {
		case err == nil:
			// Money is only locked in POST /api/orders, where the promo is
			// re-checked against the full order context.
			applied = &pricing.AppliedPromo{Code: code.Code, Pct: code.DiscountPct}
		case errors.Is(err, store.ErrNotFound):
			// Unknown code: quote without the discount.
		default:
			return domain.Quote{}, err
		}
	}

	return pricing.Calc(pricing.Input{
		Product: request.Product,
		Works:   request.Works,
		Units:   request.Units,
	}, config, applied), nil
}

// PromoCheckRequest is the POST /api/promos/check body.
type PromoCheckRequest struct {
	Code         string `json:"code"`
	Product      string `json:"product"`
	Units        int    `json:"units"`
	Works        int    `json:"works"`
	IsFirstOrder bool   `json:"isFirstOrder"`
	// Subtotal is optional: when omitted the server computes it.
	Subtotal *int `json:"subtotal"`
}

// PromoCheckResult is the {ok:true,promo} | {ok:false,code,reason} union the
// frontend renders directly.
type PromoCheckResult struct {
	OK     bool              `json:"ok"`
	Promo  *domain.PromoCode `json:"promo,omitempty"`
	Code   string            `json:"code,omitempty"`
	Reason string            `json:"reason,omitempty"`
}

// CheckPromo validates a promo code for the current form state.
func (s *Service) CheckPromo(ctx context.Context, request PromoCheckRequest) (PromoCheckResult, error) {
	raw := strings.TrimSpace(request.Code)
	if raw == "" {
		return failureResult(promo.Required()), nil
	}

	code, err := s.Store.GetPromoByCode(ctx, raw)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return failureResult(promo.NotFound()), nil
		}
		return PromoCheckResult{}, err
	}

	contextValue := promo.Context{
		IsFirstOrder: request.IsFirstOrder,
		Product:      request.Product,
		Units:        request.Units,
	}
	if request.Subtotal != nil {
		contextValue.Subtotal = *request.Subtotal
	} else {
		config, err := s.Store.GetPricing(ctx)
		if err != nil {
			return PromoCheckResult{}, err
		}
		quote := pricing.Calc(pricing.Input{
			Product: request.Product,
			Works:   maxInt(request.Works, 1),
			Units:   request.Units,
		}, config, nil)
		contextValue.Subtotal = quote.Subtotal
	}

	if failure := promo.Check(&code, contextValue, nowUTC()); failure != nil {
		return failureResult(failure), nil
	}
	return PromoCheckResult{OK: true, Promo: &code}, nil
}

// resolvePromo re-checks the code sent with a new order. It returns the promo
// to apply (nil when the code no longer qualifies) and the drop reason.
func (s *Service) resolvePromo(ctx context.Context, rawCode string, quote domain.Quote, isFirstOrder bool) (*domain.PromoCode, string) {
	if strings.TrimSpace(rawCode) == "" {
		return nil, ""
	}
	code, err := s.Store.GetPromoByCode(ctx, rawCode)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, promo.NotFound().Reason
		}
		s.Log.Error("promo lookup failed", "code", rawCode, "error", err)
		return nil, "Не удалось проверить промокод, заказ создан без скидки"
	}
	failure := promo.Check(&code, promo.Context{
		IsFirstOrder: isFirstOrder,
		Product:      quote.Product,
		Units:        quote.Units,
		Subtotal:     quote.Subtotal,
	}, nowUTC())
	if failure != nil {
		return nil, failure.Reason
	}
	return &code, ""
}

func failureResult(failure *promo.Failure) PromoCheckResult {
	return PromoCheckResult{OK: false, Code: failure.Code, Reason: failure.Reason}
}

// IsFirstOrder decides the `appliesTo = first` rule. The mock used "this
// browser has no saved orders"; the backend keys the check on the Telegram
// contact once the order is linked to the bot, and falls back to the number of
// orders the browser still remembers.
func (s *Service) IsFirstOrder(ctx context.Context, contact string, knownOrders int) bool {
	if contact != "" {
		count, err := s.Store.CountOrdersByContact(ctx, contact)
		if err != nil {
			s.Log.Error("first-order check failed", "error", err)
			return knownOrders == 0
		}
		return count == 0
	}
	return knownOrders == 0
}

// Products returns the catalogue (names, unit words, ranges).
func Products() []catalog.Product { return catalog.Products }

// Stats is GET /api/admin/stats: order counts, revenue, queue health, disk.
func (s *Service) Stats(ctx context.Context) (domain.Stats, error) {
	stats, err := s.Store.Stats(ctx)
	if err != nil {
		return stats, err
	}
	stats.DiskUsedBytes = s.DiskUsage()
	// The generation spend is billed in USD: show it in tenge too and tell the
	// operator what the revenue is after it (every run counts, including the
	// failed ones - that money is spent as well).
	stats.USDRate = s.USDRate(ctx)
	stats.AgentCostKZT = float64(int(stats.AgentCostUSD*stats.USDRate + 0.5))
	stats.NetRevenue = stats.ConfirmedRevenue - int(stats.AgentCostKZT)
	return stats, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
