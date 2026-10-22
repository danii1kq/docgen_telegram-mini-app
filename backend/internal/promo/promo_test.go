package promo

import (
	"testing"
	"time"

	"github.com/srunal/srunal-go/internal/domain"
)

func intPtr(value int) *int { return &value }

func timePtr(value time.Time) *time.Time { return &value }

// basePromo is an unrestricted active code.
func basePromo() domain.PromoCode {
	return domain.PromoCode{
		ID: "p1", Code: "SRUNAL10", DiscountPct: 10, AppliesTo: "any",
		Products: []string{}, Active: true, CreatedAt: time.Now().UTC(),
	}
}

// TestCheckOrderOfMessages walks the documented validation order (README 5.4).
func TestCheckOrderOfMessages(t *testing.T) {
	now := time.Now().UTC()
	context := Context{IsFirstOrder: false, Product: "essay", Units: 10, Subtotal: 800}

	disabled := basePromo()
	disabled.Active = false
	if failure := Check(&disabled, context, now); failure == nil || failure.Reason != "Промокод отключён" {
		t.Errorf("disabled: %+v", failure)
	}

	expired := basePromo()
	expired.ExpiresAt = timePtr(now.Add(-time.Hour))
	if failure := Check(&expired, context, now); failure == nil || failure.Reason != "Срок действия промокода истёк" {
		t.Errorf("expired: %+v", failure)
	}

	exhausted := basePromo()
	exhausted.UsageLimit = intPtr(30)
	exhausted.UsedCount = 30
	if failure := Check(&exhausted, context, now); failure == nil || failure.Reason != "Лимит использования промокода исчерпан" {
		t.Errorf("limit: %+v", failure)
	}

	firstOnly := basePromo()
	firstOnly.AppliesTo = "first"
	if failure := Check(&firstOnly, context, now); failure == nil || failure.Reason != "Промокод действует только на первый заказ" {
		t.Errorf("first order: %+v", failure)
	}
	if failure := Check(&firstOnly, Context{IsFirstOrder: true, Product: "essay", Units: 10, Subtotal: 800}, now); failure != nil {
		t.Errorf("first order should pass for a new client: %+v", failure)
	}

	labOnly := basePromo()
	labOnly.Products = []string{"lab", "presentation"}
	if failure := Check(&labOnly, context, now); failure == nil ||
		failure.Reason != "Промокод действует только на «Лабораторная», «Презентация»" {
		t.Errorf("product mismatch: %+v", failure)
	}

	minUnits := basePromo()
	minUnits.MinUnits = intPtr(15)
	// essay -> "страниц"
	if failure := Check(&minUnits, context, now); failure == nil ||
		failure.Reason != "Промокод действует от 15 страниц" {
		t.Errorf("min units (essay): %+v", failure)
	}
	// presentation -> "слайдов"
	if failure := Check(&minUnits, Context{Product: "presentation", Units: 10, Subtotal: 2400}, now); failure == nil ||
		failure.Reason != "Промокод действует от 15 слайдов" {
		t.Errorf("min units (presentation): %+v", failure)
	}

	minAmount := basePromo()
	minAmount.MinAmount = intPtr(1000)
	if failure := Check(&minAmount, Context{Product: "essay", Units: 10, Subtotal: 800}, now); failure == nil ||
		failure.Reason != "Промокод действует от 1\u00a0000 ₸" {
		t.Errorf("min amount: %+v", failure)
	}

	if failure := Check(&minAmount, Context{Product: "essay", Units: 20, Subtotal: 1600}, now); failure != nil {
		t.Errorf("valid code rejected: %+v", failure)
	}
}

// TestCheckExposesMachineCodes checks the codes the frontend may switch to.
func TestCheckExposesMachineCodes(t *testing.T) {
	now := time.Now().UTC()
	disabled := basePromo()
	disabled.Active = false
	failure := Check(&disabled, Context{Product: "essay", Units: 10}, now)
	if failure == nil || failure.Code != CodeDisabled {
		t.Fatalf("expected code %q, got %+v", CodeDisabled, failure)
	}

	minUnits := basePromo()
	minUnits.MinUnits = intPtr(5)
	failure = Check(&minUnits, Context{Product: "essay", Units: 4}, now)
	if failure == nil || failure.Code != CodeMinUnits {
		t.Fatalf("expected code %q, got %+v", CodeMinUnits, failure)
	}
}
