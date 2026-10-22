package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/validate"
)

// ListPromos is GET /api/admin/promos.
func (s *Service) ListPromos(ctx context.Context) ([]domain.PromoCode, error) {
	return s.Store.ListPromos(ctx)
}

// CreatePromo is POST /api/admin/promos.
func (s *Service) CreatePromo(ctx context.Context, input domain.PromoInput) (domain.PromoCode, error) {
	if input.AppliesTo == "" {
		input.AppliesTo = "any"
	}
	if input.Active == nil {
		active := true
		input.Active = &active
	}
	if fields := validate.Promo(&input); len(fields) > 0 {
		return domain.PromoCode{}, ValidationError(fields)
	}
	if _, err := s.Store.GetPromoByCode(ctx, input.Code); err == nil {
		return domain.PromoCode{}, duplicatePromo(store.NormalizePromoCode(input.Code))
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.PromoCode{}, err
	}

	promo := domain.PromoCode{
		Code:        store.NormalizePromoCode(input.Code),
		DiscountPct: input.DiscountPct,
		AppliesTo:   input.AppliesTo,
		Products:    input.Products,
		MinAmount:   input.MinAmount,
		MinUnits:    input.MinUnits,
		UsageLimit:  input.UsageLimit,
		ExpiresAt:   input.ExpiresAt,
		Active:      *input.Active,
	}
	created, err := s.Store.CreatePromo(ctx, promo)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.PromoCode{}, duplicatePromo(promo.Code)
		}
		return domain.PromoCode{}, err
	}
	return created, nil
}

// UpdatePromo is PATCH /api/admin/promos/{id}. Only the fields present in the
// JSON body are changed (the mock sent full objects; a partial patch from the
// enable/disable switch must not wipe the rest).
func (s *Service) UpdatePromo(ctx context.Context, id string, patch domain.PromoInput) (domain.PromoCode, error) {
	current, err := s.Store.GetPromoByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.PromoCode{}, NotFound("Промокод не найден")
		}
		return domain.PromoCode{}, err
	}

	merged := domain.PromoInput{
		Code:        current.Code,
		DiscountPct: current.DiscountPct,
		AppliesTo:   current.AppliesTo,
		Products:    current.Products,
		MinAmount:   current.MinAmount,
		MinUnits:    current.MinUnits,
		UsageLimit:  current.UsageLimit,
		ExpiresAt:   current.ExpiresAt,
		Active:      &current.Active,
	}
	if patch.Code != "" {
		merged.Code = patch.Code
	}
	if patch.DiscountPct != 0 {
		merged.DiscountPct = patch.DiscountPct
	}
	if patch.AppliesTo != "" {
		merged.AppliesTo = patch.AppliesTo
	}
	if patch.Products != nil {
		merged.Products = patch.Products
	}
	if patch.MinAmount != nil {
		merged.MinAmount = patch.MinAmount
	}
	if patch.MinUnits != nil {
		merged.MinUnits = patch.MinUnits
	}
	if patch.UsageLimit != nil {
		merged.UsageLimit = patch.UsageLimit
	}
	if patch.ExpiresAt != nil {
		merged.ExpiresAt = patch.ExpiresAt
	}
	if patch.Active != nil {
		merged.Active = patch.Active
	}

	if fields := validate.Promo(&merged); len(fields) > 0 {
		return domain.PromoCode{}, ValidationError(fields)
	}
	if store.NormalizePromoCode(merged.Code) != current.Code {
		if existing, err := s.Store.GetPromoByCode(ctx, merged.Code); err == nil && existing.ID != id {
			return domain.PromoCode{}, duplicatePromo(store.NormalizePromoCode(merged.Code))
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			return domain.PromoCode{}, err
		}
	}

	updated, err := s.Store.UpdatePromo(ctx, id, merged)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.PromoCode{}, duplicatePromo(store.NormalizePromoCode(merged.Code))
		}
		if errors.Is(err, store.ErrNotFound) {
			return domain.PromoCode{}, NotFound("Промокод не найден")
		}
		return domain.PromoCode{}, err
	}
	return updated, nil
}

// DeletePromo is DELETE /api/admin/promos/{id}.
func (s *Service) DeletePromo(ctx context.Context, id string) error {
	if err := s.Store.DeletePromo(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return NotFound("Промокод не найден")
		}
		return err
	}
	return nil
}

func duplicatePromo(code string) *APIError {
	return &APIError{
		Status:  409,
		Code:    CodeConflict,
		Message: fmt.Sprintf("Промокод %s уже существует", code),
	}
}
