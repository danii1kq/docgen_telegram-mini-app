package store_test

import (
	"testing"

	"github.com/srunal/srunal-go/internal/seed"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

// TestBaseSeedOnEmptyDatabase checks what a production start (SEED_ON_START=false)
// does with an empty database: it must leave a working configuration - the price
// list and the settings row - and no demo content at all (orders, reviews,
// promos, operators). Quotes would be 0 ₸ without it, and the operator registry
// would show demo orders.
func TestBaseSeedOnEmptyDatabase(t *testing.T) {
	svc, ctx := testService(t) // truncates every table

	if err := seed.ImportBase(ctx, svc.Store, svc.Log); err != nil {
		t.Fatalf("ImportBase: %v", err)
	}

	pricing, err := svc.Store.GetPricing(ctx)
	if err != nil {
		t.Fatalf("GetPricing: %v", err)
	}
	if len(pricing.Products) != 4 {
		t.Fatalf("products = %d, want 4", len(pricing.Products))
	}
	if len(pricing.Discounts) != 3 {
		t.Fatalf("discounts = %d, want 3", len(pricing.Discounts))
	}
	if price, ok := pricing.UnitPriceFor("essay"); !ok || price != 80 {
		t.Fatalf("essay price = %d (%v), want 80", price, ok)
	}

	settings, err := svc.Store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.SupportTelegram == "" {
		t.Fatal("settings row has no support handle")
	}

	// No demo content: no orders, no reviews, no promos, no operators.
	orders, err := svc.Store.ListOrders(ctx, store.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if len(orders.Orders) != 0 {
		t.Fatalf("orders = %d, want 0", len(orders.Orders))
	}
	reviews, err := svc.Store.ListReviews(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) != 0 {
		t.Fatalf("reviews = %d, want 0", len(reviews))
	}
	promos, err := svc.Store.ListPromos(ctx)
	if err != nil {
		t.Fatalf("ListPromos: %v", err)
	}
	if len(promos) != 0 {
		t.Fatalf("promos = %d, want 0", len(promos))
	}
	if chats, err := svc.Store.OperatorChatIDs(ctx); err != nil || len(chats) != 0 {
		t.Fatalf("operator chats = %v (%v), want none", chats, err)
	}

	// A quote therefore works out of the box on a fresh production database.
	quote, err := svc.Quote(ctx, service.QuoteRequest{Product: "essay", Units: 10, Works: 1})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if quote.Total != 800 {
		t.Fatalf("quote total = %d, want 800", quote.Total)
	}
}
