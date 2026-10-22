package store

import (
	"context"
	"fmt"
	"time"

	"github.com/srunal/srunal-go/internal/domain"
)

// Stats aggregates the metrics of GET /api/admin/stats. Times are grouped by
// the Asia/Almaty day so "today" matches the operator's clock.
func (s *Store) Stats(ctx context.Context) (domain.Stats, error) {
	stats := domain.Stats{
		OrdersByStatus:           map[string]int{},
		OldestQueuedMinutes:      -1,
		AverageGenerationSeconds: -1,
		GeneratedAt:              time.Now().UTC(),
	}

	counts, err := s.CountByStatus(ctx)
	if err != nil {
		return stats, err
	}
	stats.OrdersByStatus = counts
	for _, count := range counts {
		stats.OrdersTotal += count
	}
	stats.QueueDepth = counts[domain.StatusQueued] + counts[domain.StatusGenerating]

	err = s.pool.QueryRow(ctx,
		`select
		   coalesce(sum(amount) filter (where status in ('queued','generating','completed')), 0),
		   coalesce(sum(amount) filter (where confirmed_at >= date_trunc('day', now() at time zone 'Asia/Almaty') at time zone 'Asia/Almaty'), 0),
		   count(*) filter (where completed_at >= date_trunc('day', now() at time zone 'Asia/Almaty') at time zone 'Asia/Almaty'),
		   coalesce(extract(epoch from (now() - min(updated_at) filter (where status = 'queued')))::float8, -1),
		   coalesce(avg(extract(epoch from (completed_at - coalesce(confirmed_at, created_at))))
		            filter (where status = 'completed' and completed_at is not null)::float8, -1),
		   coalesce(sum(agent_cost_usd), 0)
		 from orders`,
	).Scan(&stats.ConfirmedRevenue, &stats.RevenueToday, &stats.CompletedToday,
		&stats.OldestQueuedMinutes, &stats.AverageGenerationSeconds, &stats.AgentCostUSD)
	if err != nil {
		return stats, fmt.Errorf("store: stats aggregates: %w", err)
	}
	if stats.OldestQueuedMinutes > 30 {
		stats.QueueWarning = true
	}

	if stats.OrdersTotal > 0 {
		problem := counts[domain.StatusFailedFinal] + counts[domain.StatusPaymentRejected] +
			counts[domain.StatusCancelled] + counts[domain.StatusExpired]
		stats.FailureRatePct = float64(problem) * 100 / float64(stats.OrdersTotal)
	}

	if err := s.pool.QueryRow(ctx,
		`select count(*) from reviews where status = 'pending'`,
	).Scan(&stats.PendingReviews); err != nil {
		return stats, fmt.Errorf("store: pending reviews: %w", err)
	}
	activePromos, err := s.CountActivePromos(ctx)
	if err != nil {
		return stats, err
	}
	stats.ActivePromos = activePromos
	return stats, nil
}
