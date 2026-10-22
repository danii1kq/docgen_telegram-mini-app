package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/srunal/srunal-go/internal/domain"
)

const reviewColumns = `id, order_number, author, institute, department, discipline,
	product_code, rating::float8, text, status, created_at`

func scanReview(row rowScanner) (domain.Review, error) {
	var review domain.Review
	err := row.Scan(&review.ID, &review.OrderNumber, &review.Author, &review.Institute,
		&review.Department, &review.Discipline, &review.Product, &review.Rating,
		&review.Text, &review.Status, &review.CreatedAt)
	if err != nil {
		return review, err
	}
	review.CreatedAt = utc(review.CreatedAt)
	return review, nil
}

// ListReviews returns reviews newest first. status may be empty (all).
func (s *Store) ListReviews(ctx context.Context, status string, limit, offset int) ([]domain.Review, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args := []any{}
	where := "1 = 1"
	if status != "" {
		args = append(args, status)
		where = "status = $1"
	}
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx,
		`select `+reviewColumns+` from reviews where `+where+`
		  order by created_at desc
		  limit $`+itoa(len(args)-1)+` offset $`+itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("store: list reviews: %w", err)
	}
	defer rows.Close()

	reviews := []domain.Review{}
	for rows.Next() {
		review, err := scanReview(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan review: %w", err)
		}
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}

// ReviewCounts returns the per-status counters for the moderation tabs.
func (s *Store) ReviewCounts(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{}
	rows, err := s.pool.Query(ctx, `select status, count(*) from reviews group by status`)
	if err != nil {
		return counts, fmt.Errorf("store: review counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return counts, fmt.Errorf("store: scan review count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// CreateReview inserts a pending review. The caller verifies the order exists.
func (s *Store) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	err := s.pool.QueryRow(ctx,
		`insert into reviews (id, order_number, author, institute, department, discipline,
		        product_code, rating, text, status, created_at)
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		 returning `+reviewColumns,
		review.ID, review.OrderNumber, review.Author, review.Institute, review.Department,
		review.Discipline, review.Product, review.Rating, review.Text,
		domain.ReviewPending,
	).Scan(&review.ID, &review.OrderNumber, &review.Author, &review.Institute,
		&review.Department, &review.Discipline, &review.Product, &review.Rating,
		&review.Text, &review.Status, &review.CreatedAt)
	if err != nil {
		return review, classify(fmt.Errorf("store: insert review: %w", err))
	}
	review.CreatedAt = utc(review.CreatedAt)
	return review, nil
}

// UpdateReviewStatus publishes or rejects a review.
func (s *Store) UpdateReviewStatus(ctx context.Context, id, status, operatorID string) (domain.Review, error) {
	var review domain.Review
	err := s.pool.QueryRow(ctx,
		`update reviews set status = $2, moderated_at = now(), moderated_by = $3
		  where id = $1
		  returning `+reviewColumns,
		id, status, operatorID,
	).Scan(&review.ID, &review.OrderNumber, &review.Author, &review.Institute,
		&review.Department, &review.Discipline, &review.Product, &review.Rating,
		&review.Text, &review.Status, &review.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return review, ErrNotFound
	}
	if err != nil {
		return review, fmt.Errorf("store: update review: %w", err)
	}
	review.CreatedAt = utc(review.CreatedAt)
	return review, nil
}

// OrderExists checks that a review may reference the order.
func (s *Store) OrderExists(ctx context.Context, number string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`select exists(select 1 from orders where number = $1)`, number).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: order exists: %w", err)
	}
	return exists, nil
}

// HasReviewForOrder reports whether the order already has a review.
func (s *Store) HasReviewForOrder(ctx context.Context, number string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`select exists(select 1 from reviews where order_number = $1 and status <> 'rejected')`,
		number).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: review exists: %w", err)
	}
	return exists, nil
}
