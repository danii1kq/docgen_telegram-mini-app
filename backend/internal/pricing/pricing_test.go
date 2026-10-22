package pricing

import (
	"strings"
	"testing"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
)

// defaultConfig mirrors the seeded price table (README section 4).
func defaultConfig() domain.PricingConfig {
	return domain.PricingConfig{
		Products: []domain.PricingProduct{
			{Code: "essay", Min: 4, Max: 40, UnitPrice: 80, Active: true},
			{Code: "lab", Min: 1, Max: 1, UnitPrice: 1000, Active: true},
			{Code: "presentation", Min: 5, Max: 30, UnitPrice: 240, Active: true},
			{Code: "other", Min: 1, Max: 1, UnitPrice: 1000, Active: true},
		},
		Discounts: []domain.VolumeDiscount{
			{Min: 5, Pct: 10},
			{Min: 10, Pct: 15},
			{Min: 20, Pct: 20},
		},
	}
}

// TestCalcQuote checks the documented formula and the demo plan from the README.
func TestCalcQuote(t *testing.T) {
	config := defaultConfig()

	quote := Calc(Input{Product: "essay", Works: 1, Units: 10}, config, nil)
	if quote.Subtotal != 800 || quote.Total != 800 {
		t.Fatalf("essay 10 pages: subtotal=%d total=%d, want 800/800", quote.Subtotal, quote.Total)
	}
	if len(quote.Lines) < 5 {
		t.Fatalf("expected the full breakdown, got %d lines", len(quote.Lines))
	}
	if quote.Lines[0].Label != "Реферат (базовый тариф)" {
		t.Errorf("price line label = %q", quote.Lines[0].Label)
	}
	if quote.Lines[0].Value != "80 ₸ / стр." {
		t.Errorf("price line value = %q, want '80 ₸ / стр.'", quote.Lines[0].Value)
	}
	if quote.Lines[1].Label != "Страницы (10 × 80 ₸)" || quote.Lines[1].Value != "800 ₸" {
		t.Errorf("volume line = %q / %q", quote.Lines[1].Label, quote.Lines[1].Value)
	}
	if quote.Lines[3].Value != "1 шт." {
		t.Errorf("works line = %q", quote.Lines[3].Value)
	}
	if quote.Lines[4].Value != "-" {
		t.Errorf("volume discount line = %q, want '-'", quote.Lines[4].Value)
	}
}

// TestVolumeTiers checks that only the highest reached tier applies.
func TestVolumeTiers(t *testing.T) {
	config := defaultConfig()
	cases := []struct {
		works      int
		pct        int
		total      int
		discounted int
	}{
		{works: 1, pct: 0, total: 800},
		{works: 4, pct: 0, total: 3200},
		{works: 5, pct: 10, total: 3600, discounted: 400},
		{works: 10, pct: 15, total: 6800, discounted: 1200},
		{works: 20, pct: 20, total: 12800, discounted: 3200},
	}
	for _, testCase := range cases {
		quote := Calc(Input{Product: "essay", Works: testCase.works, Units: 10}, config, nil)
		if quote.DiscountPct != testCase.pct || quote.Total != testCase.total {
			t.Errorf("works=%d: pct=%d total=%d, want pct=%d total=%d",
				testCase.works, quote.DiscountPct, quote.Total, testCase.pct, testCase.total)
		}
		if testCase.discounted != 0 && quote.Discount != testCase.discounted {
			t.Errorf("works=%d: discount=%d, want %d", testCase.works, quote.Discount, testCase.discounted)
		}
	}
}

// TestPromoAppliedAfterVolume checks the promo applies to the post-volume sum.
func TestPromoAppliedAfterVolume(t *testing.T) {
	config := defaultConfig()
	quote := Calc(Input{Product: "essay", Works: 10, Units: 10}, config, &AppliedPromo{Code: "SRUNAL10", Pct: 10})
	// 8000 - 15% = 6800, then -10% = 6120
	if quote.DiscountPct != 15 || quote.Discount != 1200 {
		t.Fatalf("volume part wrong: pct=%d discount=%d", quote.DiscountPct, quote.Discount)
	}
	if quote.PromoPct != 10 || quote.PromoDiscount != 680 || quote.Total != 6120 {
		t.Fatalf("promo part wrong: pct=%d discount=%d total=%d", quote.PromoPct, quote.PromoDiscount, quote.Total)
	}
	if quote.PromoCode == nil || *quote.PromoCode != "SRUNAL10" {
		t.Fatalf("promo code missing: %v", quote.PromoCode)
	}
	last := quote.Lines[len(quote.Lines)-1]
	if last.Label != "Промокод SRUNAL10" || last.Value != "-10% (-680 ₸)" || !last.Discount {
		t.Errorf("promo line = %+v", last)
	}
}

// TestFixedProducts checks that fixed products ignore the units stepper.
func TestFixedProducts(t *testing.T) {
	config := defaultConfig()
	for _, code := range []string{"lab", "other"} {
		quote := Calc(Input{Product: code, Works: 1, Units: 17}, config, nil)
		if quote.Units != 1 || quote.Total != 1000 {
			t.Errorf("%s: units=%d total=%d, want 1/1000", code, quote.Units, quote.Total)
		}
		if quote.Lines[1].Label != "Объём" || quote.Lines[1].Hint != "выбор не требуется" {
			t.Errorf("%s: fixed volume line = %+v", code, quote.Lines[1])
		}
	}
}

// TestUnitsClamped checks the clamp to the product range (mirrors pricing.ts).
func TestUnitsClamped(t *testing.T) {
	config := defaultConfig()
	if quote := Calc(Input{Product: "essay", Works: 1, Units: 2}, config, nil); quote.Units != 4 {
		t.Errorf("units below min: got %d, want 4", quote.Units)
	}
	if quote := Calc(Input{Product: "essay", Works: 1, Units: 99}, config, nil); quote.Units != 40 {
		t.Errorf("units above max: got %d, want 40", quote.Units)
	}
	if quote := Calc(Input{Product: "presentation", Works: 0, Units: 12}, config, nil); quote.Works != 1 {
		t.Errorf("works floored to 1, got %d", quote.Works)
	}
}

// TestFallbackPrice checks that a product missing from the price table falls
// back to the catalogue price (pricing.ts: `pricingProduct?.unitPrice ?? ...`).
func TestFallbackPrice(t *testing.T) {
	quote := Calc(Input{Product: "essay", Works: 1, Units: 10}, domain.PricingConfig{}, nil)
	if quote.UnitPrice != 80 || quote.Total != 800 {
		t.Fatalf("fallback price wrong: unit=%d total=%d", quote.UnitPrice, quote.Total)
	}
	if quote.DiscountPct != 0 {
		t.Errorf("no tiers configured -> no discount, got %d", quote.DiscountPct)
	}
}

// TestRoundingKeepsIntegerKZT guards the Math.round equivalence with an odd
// combination: 7 works reach the 10% tier, then the promo applies on top.
func TestRoundingKeepsIntegerKZT(t *testing.T) {
	config := defaultConfig()
	config.Products[0].UnitPrice = 83
	quote := Calc(Input{Product: "essay", Works: 7, Units: 13}, config, &AppliedPromo{Code: "X10", Pct: 17})
	// subtotal = 83*13*7 = 7553; tier 10% -> round(755.3) = 755; after = 6798;
	// promo 17% -> round(1155.66) = 1156; total = 5642
	if quote.Subtotal != 7553 {
		t.Fatalf("subtotal = %d, want 7553", quote.Subtotal)
	}
	if quote.Discount != 755 || quote.PromoDiscount != 1156 || quote.Total != 5642 {
		t.Fatalf("rounding: discount=%d promo=%d total=%d", quote.Discount, quote.PromoDiscount, quote.Total)
	}
}

// TestMoneyFormat pins the exact rendering shared with format.ts.
func TestMoneyFormat(t *testing.T) {
	if got := format.Money(1000); got != "1\u00a0000 ₸" {
		t.Errorf("Money(1000) = %q", got)
	}
	if got := format.Money(80); got != "80 ₸" {
		t.Errorf("Money(80) = %q", got)
	}
	if got := format.Money(1234567); got != "1\u00a0234\u00a0567 ₸" {
		t.Errorf("Money(1234567) = %q", got)
	}
	if nbsp := strings.Count(format.Money(1000), "\u00a0"); nbsp != 1 {
		t.Errorf("expected one non-breaking space, got %d", nbsp)
	}
}

// TestPlural pins the Russian word forms used in messages.
func TestPlural(t *testing.T) {
	cases := map[int]string{
		1:  "страница",
		2:  "страницы",
		5:  "страниц",
		11: "страниц",
		21: "страница",
		22: "страницы",
	}
	for n, want := range cases {
		if got := format.Plural(n, "страница", "страницы", "страниц"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}
