package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/validate"
)

// ListReviews returns reviews newest first; status may be empty (all).
func (s *Service) ListReviews(ctx context.Context, status string, limit, offset int) ([]domain.Review, error) {
	return s.Store.ListReviews(ctx, status, limit, offset)
}

// ReviewCounts returns the counters for the moderation tabs.
func (s *Service) ReviewCounts(ctx context.Context) (map[string]int, error) {
	return s.Store.ReviewCounts(ctx)
}

// AddReview is POST /api/reviews: the order must exist, the review starts as
// `pending` and waits for moderation.
func (s *Service) AddReview(ctx context.Context, input domain.ReviewInput) (domain.Review, error) {
	input.OrderNumber = NormalizeNumber(input.OrderNumber)
	if fields := validate.Review(&input); len(fields) > 0 {
		return domain.Review{}, ValidationError(fields)
	}

	exists, err := s.Store.OrderExists(ctx, input.OrderNumber)
	if err != nil {
		return domain.Review{}, err
	}
	if !exists {
		return domain.Review{}, ValidationError(map[string]string{
			"orderNumber": "Заказ с таким номером не найден. Номер указан в чеке или Telegram-боте.",
		})
	}

	already, err := s.Store.HasReviewForOrder(ctx, input.OrderNumber)
	if err != nil {
		return domain.Review{}, err
	}
	if already {
		return domain.Review{}, ValidationError(map[string]string{
			"orderNumber": "По этому заказу уже есть отзыв",
		})
	}

	review := domain.Review{
		ID:          "r-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		OrderNumber: input.OrderNumber,
		Author:      strings.TrimSpace(input.Author),
		Institute:   strings.TrimSpace(input.Institute),
		Department:  strings.TrimSpace(input.Department),
		Discipline:  strings.TrimSpace(input.Discipline),
		Product:     input.Product,
		Rating:      input.Rating,
		Text:        strings.TrimSpace(input.Text),
		Status:      domain.ReviewPending,
		CreatedAt:   nowUTC(),
	}
	if review.Institute == "" {
		review.Institute = "Satbayev Univ"
	}
	if review.Department == "" {
		review.Department = "-"
	}
	return s.Store.CreateReview(ctx, review)
}

// ModerateReview is PATCH /api/admin/reviews/{id}.
func (s *Service) ModerateReview(ctx context.Context, id, status, operatorID string) (domain.Review, error) {
	switch status {
	case domain.ReviewPublished, domain.ReviewRejected, domain.ReviewPending:
	default:
		return domain.Review{}, ValidationError(map[string]string{
			"status": "Недопустимый статус отзыва",
		})
	}
	review, err := s.Store.UpdateReviewStatus(ctx, id, status, operatorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Review{}, NotFound("Отзыв не найден")
		}
		return domain.Review{}, fmt.Errorf("service: moderate review: %w", err)
	}
	return review, nil
}
