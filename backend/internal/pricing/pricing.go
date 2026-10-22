// Package pricing is the Go mirror of src/lib/pricing.ts - the price formula
// and the human-readable quote lines. The client never sends a price.
package pricing

import (
	"math"
	"strconv"

	"github.com/srunal/srunal-go/internal/catalog"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/format"
)

// AppliedPromo is the discount a checked promo code contributes.
type AppliedPromo struct {
	Code string
	Pct  int
}

// Input is the POST /api/quote body (QuoteInput in pricing.ts).
type Input struct {
	Product string
	Works   int
	Units   int
}

// Calc computes the quote exactly like calcQuote() in pricing.ts:
//
//	subtotal        = unit_price × units × works
//	after_volume    = subtotal - round(subtotal × tier_pct / 100)
//	promo_discount  = round(after_volume × promo_pct / 100)
//	total           = after_volume - promo_discount
func Calc(input Input, config domain.PricingConfig, promo *AppliedPromo) domain.Quote {
	product := catalog.Get(input.Product)

	unitPrice := product.PricePerUnit
	if configured, ok := config.UnitPriceFor(input.Product); ok {
		unitPrice = configured
	}

	units := 1
	if !product.Fixed {
		units = clamp(input.Units, product.Min, product.Max)
	}
	works := input.Works
	if works < 1 {
		works = 1
	}

	subtotal := unitPrice * units * works
	discountPct := config.TierFor(works)
	discount := applyPct(subtotal, discountPct)
	afterVolume := subtotal - discount

	promoPct := 0
	promoCode := ""
	if promo != nil {
		promoPct = promo.Pct
		promoCode = promo.Code
	}
	promoDiscount := 0
	if promoPct > 0 {
		promoDiscount = applyPct(afterVolume, promoPct)
	}
	total := afterVolume - promoDiscount

	lines := []domain.QuoteLine{
		{
			Label: product.Name + " (базовый тариф)",
			Value: format.Money(unitPrice) + " / " + product.UnitLabel(),
		},
	}

	if product.Fixed {
		lines = append(lines, domain.QuoteLine{
			Label: "Объём",
			Value: "По заданию методички",
			Hint:  "выбор не требуется",
		})
	} else {
		label := format.Capitalize(product.UnitTitle) + " (" +
			strconv.Itoa(units) + " × " + format.Money(unitPrice) + ")"
		if works > 1 {
			label += " × " + strconv.Itoa(works) + " работ"
		}
		lines = append(lines, domain.QuoteLine{Label: label, Value: format.Money(subtotal)})
	}

	lines = append(lines,
		domain.QuoteLine{Label: "Форматирование по ГОСТ РК", Value: "Включено (0 ₸)"},
		domain.QuoteLine{Label: "Количество работ", Value: strconv.Itoa(works) + " шт."},
	)

	if discountPct > 0 {
		lines = append(lines, domain.QuoteLine{
			Label:    "Скидка за объём",
			Value:    "-" + strconv.Itoa(discountPct) + "% (-" + format.Money(discount) + ")",
			Discount: true,
		})
	} else {
		lines = append(lines, domain.QuoteLine{Label: "Скидка за объём", Value: "-"})
	}

	if promo != nil && promoPct > 0 {
		lines = append(lines, domain.QuoteLine{
			Label:    "Промокод " + promoCode,
			Value:    "-" + strconv.Itoa(promoPct) + "% (-" + format.Money(promoDiscount) + ")",
			Discount: true,
		})
	}

	quote := domain.Quote{
		Product:       input.Product,
		UnitPrice:     unitPrice,
		Units:         units,
		Works:         works,
		Subtotal:      subtotal,
		DiscountPct:   discountPct,
		Discount:      discount,
		PromoPct:      promoPct,
		PromoDiscount: promoDiscount,
		Total:         total,
		Lines:         lines,
	}
	if promo != nil && promoPct > 0 {
		code := promoCode
		quote.PromoCode = &code
	}
	return quote
}

// applyPct is Math.round(value × pct / 100) - the same rounding the mock uses
// (half away from zero on non-negative integers).
func applyPct(value, pct int) int {
	if pct <= 0 || value <= 0 {
		return 0
	}
	return int(math.Round(float64(value) * float64(pct) / 100))
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
