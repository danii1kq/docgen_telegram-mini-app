package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/srunal/srunal-go/internal/domain"
)

// TransitionError carries the reason a status change was refused.
type TransitionError struct {
	From string
	To   string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("store: cannot move order from %s to %s", e.From, e.To)
}

func (e *TransitionError) Is(target error) bool { return target == ErrInvalidTransition }

// lockOrder selects one order row for update inside the transaction.
func lockOrder(ctx context.Context, tx pgx.Tx, number string) (*domain.Order, error) {
	row := tx.QueryRow(ctx, `select `+orderCols("o.")+` from orders o where o.number = $1 for update of o`, number)
	order, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: lock order: %w", err)
	}
	return order, nil
}

// ClaimPayment records «Я оплатил»: awaiting_payment|payment_rejected ->
// payment_claimed with the Kaspi sender name.
func (s *Store) ClaimPayment(ctx context.Context, number, tokenHash, senderName string) (*domain.Order, error) {
	return s.claimPayment(ctx, number, senderName, func(order *domain.Order) error {
		if order.TokenHash != tokenHash {
			return ErrWrongToken
		}
		return nil
	})
}

// ClaimPaymentForChat records «Я оплатил» for the Mini App, where the proof is
// the Telegram chat identity (initData) rather than the order token. A foreign
// chat gets the same neutral mismatch error as a wrong token, so ownership is
// never revealed.
func (s *Store) ClaimPaymentForChat(ctx context.Context, number string, chatID int64, senderName string) (*domain.Order, error) {
	return s.claimPayment(ctx, number, senderName, func(order *domain.Order) error {
		if order.TelegramChatID != chatID {
			return ErrWrongToken
		}
		return nil
	})
}

// claimPayment is the shared transaction of both «Я оплатил» variants: the
// authorize callback decides which identity has to match.
func (s *Store) claimPayment(ctx context.Context, number, senderName string, authorize func(*domain.Order) error) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if err := authorize(order); err != nil {
			return err
		}
		if order.Status != domain.StatusAwaitingPayment && order.Status != domain.StatusPaymentRejected {
			return &TransitionError{From: order.Status, To: domain.StatusPaymentClaimed}
		}

		at := time.Now().UTC()
		event := "Клиент нажал «Я оплатил» (отправитель: " + senderName + ")"
		clientNote := "Клиент нажал «Я оплатил». Отправитель: " + senderName
		row := tx.QueryRow(ctx,
			`update orders
			    set status = $2, sender_name = $3, claimed_at = $4, client_note = $5,
			        reject_reason = null, updated_at = $4
			  where number = $1
			  returning `+orderCols(""),
			number, domain.StatusPaymentClaimed, senderName, at, clientNote)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: claim payment: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorClient, event, at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// ConfirmPayment: payment_claimed -> queued (operator verified the transfer).
func (s *Store) ConfirmPayment(ctx context.Context, number, operatorID string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusPaymentClaimed {
			return &TransitionError{From: order.Status, To: domain.StatusQueued}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set status = $2, confirmed_at = $3, operator_note = '', updated_at = $3
			  where number = $1 returning `+orderCols(""),
			number, domain.StatusQueued, at)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: confirm payment: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorOperator,
			"Оплата подтверждена, заказ поставлен в очередь", at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// RejectPayment: payment_claimed -> payment_rejected with a visible reason.
func (s *Store) RejectPayment(ctx context.Context, number, reason string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusPaymentClaimed {
			return &TransitionError{From: order.Status, To: domain.StatusPaymentRejected}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set status = $2, reject_reason = $3, updated_at = $4
			  where number = $1 returning `+orderCols(""),
			number, domain.StatusPaymentRejected, reason, at)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: reject payment: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorOperator, "Оплата отклонена: "+reason, at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

var cancellableStatuses = []string{
	domain.StatusAwaitingPayment, domain.StatusPaymentClaimed,
	domain.StatusPaymentRejected, domain.StatusQueued,
}

// CancelOrder: awaiting_payment|payment_claimed|payment_rejected|queued -> cancelled.
func (s *Store) CancelOrder(ctx context.Context, number, reason string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if !containsString(cancellableStatuses, order.Status) {
			return &TransitionError{From: order.Status, To: domain.StatusCancelled}
		}
		at := time.Now().UTC()
		text := "Заказ отменён оператором"
		if reason != "" {
			text += ": " + reason
		}
		row := tx.QueryRow(ctx,
			`update orders set status = $2, updated_at = $3 where number = $1 returning `+orderCols(""),
			number, domain.StatusCancelled, at)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: cancel order: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorOperator, text, at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// RetryGeneration: failed_final|payment_rejected -> queued with a new attempt.
func (s *Store) RetryGeneration(ctx context.Context, number string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusFailedFinal && order.Status != domain.StatusPaymentRejected {
			return &TransitionError{From: order.Status, To: domain.StatusQueued}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set status = $2, confirmed_at = $3, reject_reason = null, updated_at = $3
			  where number = $1 returning `+orderCols(""),
			number, domain.StatusQueued, at)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: retry generation: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorOperator, "Повторная генерация запущена", at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// RequestRevision records the corrections the client asks for on a finished
// order (the status stays: the work is still the delivered one) and flags it for
// the operator.
func (s *Store) RequestRevision(ctx context.Context, number, note string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusCompleted && order.Status != domain.StatusFailedFinal {
			return &TransitionError{From: order.Status, To: order.Status}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set revision_note = $2, revision_requested_at = $3, updated_at = $3
			  where number = $1 returning `+orderCols(""),
			number, note, at)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: request revision: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorClient, "Клиент просит внести правки", at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// ReworkOrder sends a finished order back to the agent with the (edited)
// corrections: completed|failed_final -> queued.
func (s *Store) ReworkOrder(ctx context.Context, number, note string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusCompleted && order.Status != domain.StatusFailedFinal {
			return &TransitionError{From: order.Status, To: domain.StatusQueued}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set status = $2, revision_note = $3, revision_requested_at = null,
			        reject_reason = null, completed_at = null, updated_at = $4
			  where number = $1 returning `+orderCols(""),
			number, domain.StatusQueued, note, at)
		updated, err = scanOrder(row)
		if err != nil {
			return fmt.Errorf("store: rework order: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorOperator, "Отправлено на доработку: "+clip(note, 300), at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// AddAgentCost adds the cost of one generation run to the order total (USD).
func (s *Store) AddAgentCost(ctx context.Context, number string, usd float64) error {
	if usd <= 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx,
		`update orders set agent_cost_usd = agent_cost_usd + $2 where number = $1`,
		number, usd); err != nil {
		return fmt.Errorf("store: add agent cost: %w", err)
	}
	return nil
}

func clip(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// SaveOperatorNote stores the private note of the operator.
func (s *Store) SaveOperatorNote(ctx context.Context, number, note string) (*domain.Order, error) {	tag, err := s.pool.Exec(ctx,
		`update orders set operator_note = $2, updated_at = now() where number = $1`, number, note)
	if err != nil {
		return nil, fmt.Errorf("store: save operator note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetOrder(ctx, number)
}

// ---------------------------------------------------------------------------
// background worker
// ---------------------------------------------------------------------------

// TakeNextQueued locks the oldest queued order, moves it to generating and
// opens a job row. ErrNotFound means the queue is empty.
func (s *Store) TakeNextQueued(ctx context.Context) (*domain.Order, int64, error) {
	var order *domain.Order
	var jobID int64
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`select `+orderCols("o.")+` from orders o
			  where o.status = $1
			  order by o.confirmed_at asc nulls first, o.created_at asc
			  for update skip locked limit 1`, domain.StatusQueued)
		found, err := scanOrder(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("store: take queued: %w", err)
		}

		var attempt int
		if err := tx.QueryRow(ctx,
			`select coalesce(max(attempt), 0) + 1 from order_jobs where order_number = $1`,
			found.Number).Scan(&attempt); err != nil {
			return fmt.Errorf("store: next attempt: %w", err)
		}

		at := time.Now().UTC()
		row = tx.QueryRow(ctx,
			`update orders set status = $2, updated_at = $3 where number = $1 returning `+orderCols(""),
			found.Number, domain.StatusGenerating, at)
		if order, err = scanOrder(row); err != nil {
			return fmt.Errorf("store: start generating: %w", err)
		}

		if err := tx.QueryRow(ctx,
			`insert into order_jobs (order_number, attempt, status, started_at)
			 values ($1, $2, 'running', $3) returning id`,
			found.Number, attempt, at).Scan(&jobID); err != nil {
			return fmt.Errorf("store: open job: %w", err)
		}
		return insertEvent(ctx, tx, found.Number, domain.ActorSystem, "Генерация документа запущена", at)
	})
	if err != nil {
		return nil, 0, err
	}
	// The caller reloads the order with events and attachments.
	return order, jobID, nil
}

// FinishJob closes a job row with a log line.
func (s *Store) FinishJob(ctx context.Context, jobID int64, status, logText string) error {
	_, err := s.pool.Exec(ctx,
		`update order_jobs set status = $2, log = $3, finished_at = now() where id = $1`,
		jobID, status, logText)
	if err != nil {
		return fmt.Errorf("store: finish job: %w", err)
	}
	return nil
}

// OrderJob is one generation attempt: the admin console shows the latest one.
type OrderJob struct {
	ID         int64
	Number     string
	Attempt    int
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
	Log        string
}

// LatestJob returns the most recent generation attempt of an order. When status
// is not empty only attempts with that status are considered (the console shows
// the attempt that completed the order, not a later service retry).
func (s *Store) LatestJob(ctx context.Context, number, status string) (OrderJob, error) {
	query := `select id, order_number, attempt, status, started_at, finished_at, coalesce(log, '')
	            from order_jobs where order_number = $1`
	args := []any{number}
	if status != "" {
		query += ` and status = $2`
		args = append(args, status)
	}
	query += ` order by attempt desc limit 1`

	var job OrderJob
	err := s.pool.QueryRow(ctx, query, args...).
		Scan(&job.ID, &job.Number, &job.Attempt, &job.Status, &job.StartedAt, &job.FinishedAt, &job.Log)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderJob{}, ErrNotFound
		}
		return OrderJob{}, fmt.Errorf("store: latest job: %w", err)
	}
	return job, nil
}

// CompleteGeneration: generating -> completed with the ready documents (one per
// work when the order has several).
func (s *Store) CompleteGeneration(ctx context.Context, number string, files []OrderFileRow, logText string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusGenerating {
			return &TransitionError{From: order.Status, To: domain.StatusCompleted}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set status = $2, completed_at = $3, updated_at = $3
			  where number = $1 returning `+orderCols(""),
			number, domain.StatusCompleted, at)
		if updated, err = scanOrder(row); err != nil {
			return fmt.Errorf("store: complete order: %w", err)
		}

		for _, file := range files {
			if file.FileName == "" {
				continue
			}
			attempt := file.Attempt
			if attempt < 1 {
				attempt = 1
			}
			if _, err := tx.Exec(ctx,
				`insert into order_files (order_number, kind, file_name, size_bytes, mime_type, storage_key, attempt, expires_at)
				 values ($1, 'result', $2, $3, $4, $5, $6, $7)`,
				number, file.FileName, file.SizeBytes, file.MimeType, file.StorageKey, attempt, file.ExpiresAt,
			); err != nil {
				return fmt.Errorf("store: insert result file: %w", err)
			}
		}
		event := "Результат готов и доступен для скачивания"
		if len(files) > 1 {
			event = fmt.Sprintf("Результат готов: %d документа(ов) доступны для скачивания", len(files))
		}
		return insertEvent(ctx, tx, number, domain.ActorSystem, event, at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// FailGeneration: generating -> failed_final with a job log for the operator.
func (s *Store) FailGeneration(ctx context.Context, number, logText string) (*domain.Order, error) {
	var updated *domain.Order
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, number)
		if err != nil {
			return err
		}
		if order.Status != domain.StatusGenerating {
			return &TransitionError{From: order.Status, To: domain.StatusFailedFinal}
		}
		at := time.Now().UTC()
		row := tx.QueryRow(ctx,
			`update orders set status = $2, updated_at = $3 where number = $1 returning `+orderCols(""),
			number, domain.StatusFailedFinal, at)
		if updated, err = scanOrder(row); err != nil {
			return fmt.Errorf("store: fail order: %w", err)
		}
		return insertEvent(ctx, tx, number, domain.ActorSystem,
			"Не удалось сгенерировать документ, заказ отмечен как проблемный", at)
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, updated.Number)
}

// RequeueStaleGenerating returns orders stuck in generating back to the queue.
func (s *Store) RequeueStaleGenerating(ctx context.Context, olderThan time.Duration) ([]string, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	rows, err := s.pool.Query(ctx,
		`update orders set status = $1, updated_at = now()
		  where status = $2 and updated_at < $3
		  returning number`, domain.StatusQueued, domain.StatusGenerating, cutoff)
	if err != nil {
		return nil, fmt.Errorf("store: requeue stale: %w", err)
	}
	numbers := []string{}
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: scan stale: %w", err)
		}
		numbers = append(numbers, number)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, number := range numbers {
		if err := s.AppendEvent(ctx, number, domain.ActorSystem,
			"Генерация прервана, заказ возвращён в очередь"); err != nil {
			return numbers, err
		}
		// The run died with the process: close its job row instead of leaving a
		// ghost "running" attempt behind.
		if _, err := s.pool.Exec(ctx,
			`update order_jobs set status = 'failed', log = $2, finished_at = now()
			  where order_number = $1 and status = 'running'`,
			number, "прервано: процесс генерации остановлен"); err != nil {
			return numbers, fmt.Errorf("store: close stale job: %w", err)
		}
	}
	return numbers, nil
}

// ExpireUnpaidOrders marks old awaiting_payment orders as expired and returns
// their numbers.
func (s *Store) ExpireUnpaidOrders(ctx context.Context, olderThan time.Duration) ([]string, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	rows, err := s.pool.Query(ctx,
		`update orders set status = $1, updated_at = now()
		  where status = $2 and created_at < $3
		  returning number`, domain.StatusExpired, domain.StatusAwaitingPayment, cutoff)
	if err != nil {
		return nil, fmt.Errorf("store: expire orders: %w", err)
	}
	numbers := []string{}
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: scan expired: %w", err)
		}
		numbers = append(numbers, number)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, number := range numbers {
		if err := s.AppendEvent(ctx, number, domain.ActorSystem, "Срок оплаты истёк"); err != nil {
			return numbers, err
		}
	}
	return numbers, nil
}

// AppendEvent writes one timeline entry (bot, worker, admin actions).
func (s *Store) AppendEvent(ctx context.Context, number, actor, text string) error {
	_, err := s.pool.Exec(ctx,
		`insert into order_events (order_number, actor, text, at) values ($1, $2, $3, now())`,
		number, actor, text)
	if err != nil {
		return fmt.Errorf("store: append event: %w", err)
	}
	return nil
}

// ExpireOldResultFiles deletes documents older than the retention window.
func (s *Store) ExpireOldResultFiles(ctx context.Context) ([]OrderFileRow, error) {
	rows, err := s.pool.Query(ctx,
		`delete from order_files
		  where kind = 'result' and expires_at is not null and expires_at < now()
		  returning id, order_number, kind, file_name, size_bytes, mime_type, storage_key, created_at, expires_at`)
	if err != nil {
		return nil, fmt.Errorf("store: expire result files: %w", err)
	}
	defer rows.Close()
	removed := []OrderFileRow{}
	for rows.Next() {
		var file OrderFileRow
		if err := rows.Scan(&file.ID, &file.Number, &file.Kind, &file.FileName, &file.SizeBytes,
			&file.MimeType, &file.StorageKey, &file.CreatedAt, &file.ExpiresAt); err != nil {
			return nil, fmt.Errorf("store: scan removed file: %w", err)
		}
		removed = append(removed, file)
	}
	return removed, rows.Err()
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
