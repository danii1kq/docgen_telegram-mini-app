package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Costs are billed in USD by the model provider while the operator thinks in
// tenge: the service keeps a USD -> KZT rate and hands it to the panel.
const (
	usdKZTRateKey = "usd_kzt_rate"
	// The rate is refreshed a few times a day: the National Bank publishes once
	// per working day, the fallback source is intraday.
	usdKZTRateTTL = 6 * time.Hour
	ratesTimeout  = 12 * time.Second
)

// USDKZT is the cached USD -> KZT exchange rate.
type USDKZT struct {
	Rate      float64   `json:"rate"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type rateCache struct {
	mu    sync.RWMutex
	value USDKZT
}

// usdKZTRate returns the cached rate. The first call after a restart loads the
// value persisted in app_kv, so the panel never shows a zero rate.
func (s *Service) usdKZTRate(ctx context.Context) USDKZT {
	s.rates.mu.RLock()
	cached := s.rates.value
	s.rates.mu.RUnlock()
	if cached.Rate > 0 {
		return cached
	}

	if raw, ok, err := s.Store.KVGet(ctx, usdKZTRateKey); err == nil && ok {
		var stored USDKZT
		if err := json.Unmarshal([]byte(raw), &stored); err == nil && stored.Rate > 0 {
			s.rates.mu.Lock()
			s.rates.value = stored
			s.rates.mu.Unlock()
			return stored
		}
	}
	return cached
}

// USDRate returns the USD -> KZT rate (0 when it is not known yet).
func (s *Service) USDRate(ctx context.Context) float64 {
	return s.usdKZTRate(ctx).Rate
}

// convertUSDToKZT rounds a USD amount to whole tenge with the cached rate.
func (s *Service) convertUSDToKZT(ctx context.Context, usd float64) float64 {
	rate := s.USDRate(ctx)
	if rate <= 0 || usd <= 0 {
		return 0
	}
	return float64(int(usd*rate + 0.5))
}

// RefreshUSDRate fetches a fresh rate and caches it (memory + app_kv).
func (s *Service) RefreshUSDRate(ctx context.Context) (USDKZT, error) {
	rate, source, err := fetchUSDRate(ctx)
	if err != nil {
		return USDKZT{}, err
	}
	fresh := USDKZT{Rate: rate, Source: source, UpdatedAt: time.Now().UTC()}
	s.rates.mu.Lock()
	s.rates.value = fresh
	s.rates.mu.Unlock()

	if raw, err := json.Marshal(fresh); err == nil {
		if err := s.Store.KVSet(ctx, usdKZTRateKey, string(raw)); err != nil {
			s.Log.Warn("rates: persist failed", "error", err)
		}
	}
	s.Log.Info("rates: usd/kzt updated", "rate", rate, "source", source)
	return fresh, nil
}

// StartUSDRateRefresher refreshes the rate on startup and then every TTL until
// the context is done.
func (s *Service) StartUSDRateRefresher(ctx context.Context) {
	go func() {
		if _, err := s.RefreshUSDRate(ctx); err != nil {
			s.Log.Warn("rates: first refresh failed", "error", err)
		}
		ticker := time.NewTicker(usdKZTRateTTL)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.RefreshUSDRate(ctx); err != nil {
					s.Log.Warn("rates: refresh failed", "error", err)
				}
			}
		}
	}()
}

// fetchUSDRate asks the National Bank of Kazakhstan first (the official source
// for tenge) and falls back to a public rates API.
func fetchUSDRate(ctx context.Context) (float64, string, error) {
	if rate, err := fetchNBKRate(ctx); err == nil {
		return rate, "НБ РК", nil
	} else {
		lastErr := err
		if rate, err := fetchOpenERRate(ctx); err == nil {
			return rate, "open.er-api.com", nil
		}
		return 0, "", fmt.Errorf("service: usd/kzt rate: %w", lastErr)
	}
}

// nbkRSS is the National Bank of Kazakhstan rates feed (rates_all.xml): one item
// per currency, <description> carries the tenge value of <quant> units.
type nbkRSS struct {
	Items []struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		Quant       string `xml:"quant"`
	} `xml:"channel>item"`
}

func fetchNBKRate(ctx context.Context) (float64, error) {
	body, err := httpGet(ctx, "https://nationalbank.kz/rss/rates_all.xml")
	if err != nil {
		return 0, err
	}
	return parseNBKRate(body)
}

// parseNBKRate reads the tenge value of one US dollar from the NBK feed.
func parseNBKRate(body []byte) (float64, error) {
	var feed nbkRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return 0, fmt.Errorf("nbk: parse: %w", err)
	}
	for _, item := range feed.Items {
		if !strings.EqualFold(strings.TrimSpace(item.Title), "USD") {
			continue
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(item.Description), ",", "."), 64)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("nbk: bad value %q", item.Description)
		}
		quant := 1.0
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(item.Quant), 64); err == nil && parsed > 0 {
			quant = parsed
		}
		return value / quant, nil
	}
	return 0, errors.New("nbk: usd not found")
}

func fetchOpenERRate(ctx context.Context) (float64, error) {
	body, err := httpGet(ctx, "https://open.er-api.com/v6/latest/USD")
	if err != nil {
		return 0, err
	}
	return parseOpenERRate(body)
}

// parseOpenERRate reads the KZT rate from the public JSON feed.
func parseOpenERRate(body []byte) (float64, error) {
	var payload struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("open.er-api: parse: %w", err)
	}
	rate := payload.Rates["KZT"]
	if rate <= 0 {
		return 0, errors.New("open.er-api: kzt not found")
	}
	return rate, nil
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, ratesTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SruNal/1.0 (+https://srunal-kz.duckdns.org)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
