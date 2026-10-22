// Package promo mirrors the promo-code checks of src/lib/api.ts checkPromo()
// and keeps the exact Russian messages the client shows.
package promo

import (
	"strconv"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
)

// Failure codes returned next to the human-readable reason. The frontend may
// switch to the codes later; the reason strings are identical to the mock.
const (
	CodeRequired        = "promo_required"
	CodeNotFound        = "promo_not_found"
	CodeDisabled        = "promo_disabled"
	CodeExpired         = "promo_expired"
	CodeLimitReached    = "promo_limit_reached"
	CodeFirstOrderOnly  = "promo_first_order_only"
	CodeProductMismatch = "promo_product_mismatch"
	CodeMinUnits        = "promo_min_units"
	CodeMinAmount       = "promo_min_amount"
)

// Context describes the order the code is checked against.
type Context struct {
	IsFirstOrder bool
	Product      string
	Units        int
	// Subtotal is the order sum before any discounts (unit_price × units × works).
	Subtotal int
}

// Failure is a rejected promo code: a machine code plus the client message.
type Failure struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// Required is the empty-input failure ('Введите промокод').
func Required() *Failure {
	return &Failure{Code: CodeRequired, Reason: "Введите промокод"}
}

// NotFound is the unknown-code failure.
func NotFound() *Failure {
	return &Failure{Code: CodeNotFound, Reason: "Промокод не найден"}
}

// Check evaluates a found promo code. A nil result means the code applies.
// The check order matches checkPromo() in api.ts exactly.
func Check(code *domain.PromoCode, ctx Context, now time.Time) *Failure {
	if !code.Active {
		return &Failure{Code: CodeDisabled, Reason: "Промокод отключён"}
	}
	if code.ExpiresAt != nil && code.ExpiresAt.Before(now) {
		return &Failure{Code: CodeExpired, Reason: "Срок действия промокода истёк"}
	}
	if code.UsageLimit != nil && code.UsedCount >= *code.UsageLimit {
		return &Failure{Code: CodeLimitReached, Reason: "Лимит использования промокода исчерпан"}
	}
	if code.AppliesTo == "first" && !ctx.IsFirstOrder {
		return &Failure{Code: CodeFirstOrderOnly, Reason: "Промокод действует только на первый заказ"}
	}
	if len(code.Products) > 0 && !contains(code.Products, ctx.Product) {
		names := make([]string, 0, len(code.Products))
		for _, product := range code.Products {
			names = append(names, "«"+catalog.Get(product).Name+"»")
		}
		return &Failure{
			Code:   CodeProductMismatch,
			Reason: "Промокод действует только на " + strings.Join(names, ", "),
		}
	}
	if code.MinUnits != nil && ctx.Units < *code.MinUnits {
		unit := catalog.Get(ctx.Product)
		word := format.Plural(*code.MinUnits, unit.UnitOne, unit.UnitFew, unit.UnitMany)
		return &Failure{
			Code:   CodeMinUnits,
			Reason: "Промокод действует от " + strconv.Itoa(*code.MinUnits) + " " + word,
		}
	}
	if code.MinAmount != nil && ctx.Subtotal < *code.MinAmount {
		return &Failure{
			Code:   CodeMinAmount,
			Reason: "Промокод действует от " + format.Money(*code.MinAmount),
		}
	}
	return nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
