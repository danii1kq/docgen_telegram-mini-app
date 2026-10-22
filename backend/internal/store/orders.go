package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/srunal/srunal-go/internal/domain"
)

// ErrInvalidTransition is returned when the order is not in a status that
// allows the requested action.
var ErrInvalidTransition = errors.New("store: invalid transition")

// ErrWrongToken is returned when a lookup pair does not match.
var ErrWrongToken = errors.New("store: token mismatch")

// orderColumnNames lists the order columns in scanOrder() order. The last two
// entries are server-side details (never serialized).
var orderColumnNames = []string{
	"number", "status", "sender_name", "product_code", "works", "units",
	"topics", "notes", "work_kind", "language", "institute", "department", "student_name", "discipline_code",
	"teacher_name", "contact_name", "contact_type", "contact", "amount", "quote_lines",
	"promo_code", "promo_discount_pct", "client_note", "operator_note", "reject_reason",
	"claimed_at", "confirmed_at", "completed_at", "created_at", "updated_at", "telegram_chat_id",
	"revision_note", "revision_requested_at", "agent_cost_usd",
	"token_hash",
}

// orderCols renders the column list with an optional table alias:
// orderCols("o.") -> "o.number, o.status, ..." and orderCols("") for RETURNING.
func orderCols(prefix string) string {
	parts := make([]string, 0, len(orderColumnNames))
	for _, name := range orderColumnNames {
		parts = append(parts, prefix+name)
	}
	return strings.Join(parts, ", ")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrder(row rowScanner) (*domain.Order, error) {
	var order domain.Order
	var quoteLines []byte
	var chatID *int64

	err := row.Scan(
		&order.Number, &order.Status, &order.SenderName, &order.Product, &order.Works, &order.Units,
		&order.Topics, &order.Notes, &order.WorkKind, &order.Language, &order.Institute, &order.Department,
		&order.StudentName, &order.DisciplineCode, &order.TeacherName, &order.ContactName,
		&order.ContactType, &order.Contact, &order.Amount, &quoteLines,
		&order.PromoCode, &order.PromoDiscountPct, &order.ClientNote, &order.OperatorNotes,
		&order.PaymentRejectedReason, &order.ClaimedAt, &order.ConfirmedAt, &order.CompletedAt,
		&order.CreatedAt, &order.UpdatedAt, &chatID,
		&order.RevisionNote, &order.RevisionRequestedAt, &order.AgentCostUSD,
		&order.TokenHash,
	)
	if err != nil {
		return nil, err
	}

	order.QuoteLines = []domain.QuoteLine{}
	if len(quoteLines) > 0 {
		if err := json.Unmarshal(quoteLines, &order.QuoteLines); err != nil {
			return nil, fmt.Errorf("store: decode quote lines: %w", err)
		}
	}
	order.Attachments = []domain.FileMeta{}
	order.Events = []domain.OrderEvent{}
	if chatID != nil {
		order.TelegramChatID = *chatID
	}

	order.CreatedAt = utc(order.CreatedAt)
	order.UpdatedAt = utc(order.UpdatedAt)
	order.ClaimedAt = utcPtr(order.ClaimedAt)
	order.ConfirmedAt = utcPtr(order.ConfirmedAt)
	order.CompletedAt = utcPtr(order.CompletedAt)
	order.RevisionRequestedAt = utcPtr(order.RevisionRequestedAt)
	return &order, nil
}

// OrderFileRow is a row of order_files.
type OrderFileRow struct {
	ID         int64
	Number     string
	Kind       string
	FileName   string
	SizeBytes  int64
	MimeType   string
	StorageKey string
	// Attempt is the generation run that produced the file: an order with
	// several works gets one file per work, all with the same attempt.
	Attempt   int
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// TokenPair links an order number to the hash of its access token.
type TokenPair struct {
	Number    string
	TokenHash string
}

// OrderFilter drives GET /api/admin/orders.
type OrderFilter struct {
	Status string // exact status filter, "" = all
	Query  string // number / topic / contact / student name
	Sort   string // created | updated
	Page   int
	Limit  int
}

// OrderList is the paginated admin registry plus per-status counters.
type OrderList struct {
	Orders []domain.Order
	Total  int
	Counts map[string]int
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

// NextOrderNumber allocates the next DG-<counter> value inside a transaction.
func nextOrderNumber(ctx context.Context, tx pgx.Tx) (string, error) {
	var value int64
	err := tx.QueryRow(ctx,
		`update counters set value = value + 1 where name = 'order_number' returning value`,
	).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		// First order ever: the demo counter starts right after the seeded data.
		if err := tx.QueryRow(ctx,
			`insert into counters (name, value) values ('order_number', 260391)
			 on conflict (name) do update set value = counters.value + 1
			 returning value`,
		).Scan(&value); err != nil {
			return "", fmt.Errorf("store: init counter: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("store: next order number: %w", err)
	}
	return fmt.Sprintf("DG-%d", value), nil
}

// CreateOrder inserts a new order and returns the allocated number.
// idempotencyKey may be empty; when set and already used, ErrConflict is
// returned and the caller replays the stored response.
func (s *Store) CreateOrder(
	ctx context.Context,
	order *domain.Order,
	token, tokenHash string,
	promoID *string,
	idempotencyKey string,
) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		number, err := nextOrderNumber(ctx, tx)
		if err != nil {
			return err
		}
		order.Number = number
		order.CreatedAt = time.Now().UTC()
		order.UpdatedAt = order.CreatedAt

		quoteLines, err := json.Marshal(order.QuoteLines)
		if err != nil {
			return fmt.Errorf("store: encode quote lines: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`insert into orders (
				number, token_hash, status, sender_name, product_code, works, units, topics, notes,
				work_kind, language, institute, department, student_name, discipline_code, teacher_name,
				contact_name, contact_type, contact, amount, quote_lines, promo_code,
				promo_discount_pct, client_note, operator_note, telegram_chat_id, created_at, updated_at)
			 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			         $17, $18, $19, $20, $21::jsonb, $22, $23, $24, '', $25, $26, $26)`,
			order.Number, tokenHash, order.Status, order.SenderName, order.Product, order.Works,
			order.Units, order.Topics, order.Notes, order.WorkKind, order.Language, order.Institute,
			order.Department, order.StudentName, order.DisciplineCode, order.TeacherName,
			order.ContactName, order.ContactType, order.Contact, order.Amount, quoteLines,
			order.PromoCode, order.PromoDiscountPct, order.ClientNote, nullChatID(order.TelegramChatID),
			order.CreatedAt,
		); err != nil {
			return classify(fmt.Errorf("store: insert order: %w", err))
		}

		for _, event := range order.Events {
			if err := insertEvent(ctx, tx, order.Number, event.Actor, event.Text, order.CreatedAt); err != nil {
				return err
			}
		}
		// The creation entry always exists, even when the caller sent none.
		if len(order.Events) == 0 {
			if err := insertEvent(ctx, tx, order.Number, domain.ActorClient, "Заказ создан", order.CreatedAt); err != nil {
				return err
			}
		}

		for _, file := range order.Attachments {
			if _, err := tx.Exec(ctx,
				`insert into order_files (order_number, kind, file_name, size_bytes, mime_type)
				 values ($1, 'client_material', $2, $3, $4)`,
				order.Number, file.Name, file.Size, file.Type,
			); err != nil {
				return fmt.Errorf("store: insert attachment: %w", err)
			}
		}

		if promoID != nil {
			if err := applyPromoUsage(ctx, tx, *promoID, order.Number); err != nil {
				return err
			}
		}

		if idempotencyKey != "" {
			if _, err := tx.Exec(ctx,
				`insert into idempotency_keys (key, order_number, token) values ($1, $2, $3)`,
				idempotencyKey, order.Number, token,
			); err != nil {
				return classify(fmt.Errorf("store: idempotency key: %w", err))
			}
		}
		return nil
	})
}

// applyPromoUsage increments used_count and records the redemption. Usage is
// counted when the order is created; a later cancellation does not restore it
// (documented in README section "Promo codes").
func applyPromoUsage(ctx context.Context, tx pgx.Tx, promoID, number string) error {
	var code string
	var pct int
	err := tx.QueryRow(ctx,
		`update promo_codes set used_count = used_count + 1 where id = $1
		 returning code, discount_pct`, promoID,
	).Scan(&code, &pct)
	if err != nil {
		return fmt.Errorf("store: increment promo usage: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`insert into promo_code_usages (promo_id, order_number) values ($1, $2)
		 on conflict do nothing`, promoID, number,
	); err != nil {
		return fmt.Errorf("store: promo usage row: %w", err)
	}
	return nil
}

// FindIdempotentOrder returns the order stored for an Idempotency-Key: the
// number and the original access token, so the response can be replayed.
func (s *Store) FindIdempotentOrder(ctx context.Context, key string) (string, string, error) {
	var number, token string
	err := s.pool.QueryRow(ctx,
		`select order_number, token from idempotency_keys where key = $1`, key).Scan(&number, &token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("store: idempotency lookup: %w", err)
	}
	return number, token, nil
}

// CountOrdersByContact counts orders known to belong to one client identity
// (contact name, or the Telegram handle once the bot linked the order). This
// drives the `appliesTo = first` promo rule.
func (s *Store) CountOrdersByContact(ctx context.Context, contact string) (int, error) {
	if contact == "" {
		return 0, nil
	}
	var count int
	err := s.pool.QueryRow(ctx,
		`select count(*) from orders
		  where contact = $1 or lower(contact_name) = lower($1)`, contact).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count orders by contact: %w", err)
	}
	return count, nil
}

// CountOrdersByChat counts the orders linked to one Telegram chat.
func (s *Store) CountOrdersByChat(ctx context.Context, chatID int64) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`select count(*) from orders where telegram_chat_id = $1`, chatID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count orders by chat: %w", err)
	}
	return count, nil
}

func insertEvent(ctx context.Context, tx pgx.Tx, number, actor, text string, at time.Time) error {
	if _, err := tx.Exec(ctx,
		`insert into order_events (order_number, actor, text, at) values ($1, $2, $3, $4)`,
		number, actor, text, at,
	); err != nil {
		return fmt.Errorf("store: insert event: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// read
// ---------------------------------------------------------------------------

// GetOrder loads one order with its timeline, attachments and result file.
func (s *Store) GetOrder(ctx context.Context, number string) (*domain.Order, error) {
	row := s.pool.QueryRow(ctx,
		`select `+orderCols("o.")+` from orders o where o.number = $1`, number)
	order, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get order: %w", err)
	}
	if err := s.hydrate(ctx, []*domain.Order{order}); err != nil {
		return nil, err
	}
	return order, nil
}

// GetOrderByToken loads an order only when number and token hash match.
func (s *Store) GetOrderByToken(ctx context.Context, number, tokenHash string) (*domain.Order, error) {
	row := s.pool.QueryRow(ctx,
		`select `+orderCols("o.")+` from orders o where o.number = $1 and o.token_hash = $2`,
		number, tokenHash)
	order, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWrongToken
		}
		return nil, fmt.Errorf("store: get order by token: %w", err)
	}
	if err := s.hydrate(ctx, []*domain.Order{order}); err != nil {
		return nil, err
	}
	return order, nil
}

// GetOrdersByTokens returns every order whose (number, token) pair matches.
func (s *Store) GetOrdersByTokens(ctx context.Context, pairs []TokenPair) ([]domain.Order, error) {
	if len(pairs) == 0 {
		return []domain.Order{}, nil
	}
	numbers := make([]string, 0, len(pairs))
	hashes := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		numbers = append(numbers, pair.Number)
		hashes = append(hashes, pair.TokenHash)
	}

	rows, err := s.pool.Query(ctx,
		`select `+orderCols("o.")+`
		   from orders o
		   join unnest($1::text[], $2::text[]) as pair(number, token_hash)
		     on pair.number = o.number and pair.token_hash = o.token_hash
		  order by o.created_at desc`,
		numbers, hashes,
	)
	if err != nil {
		return nil, fmt.Errorf("store: batch orders: %w", err)
	}
	defer rows.Close()

	orders, err := collectOrders(rows)
	if err != nil {
		return nil, err
	}
	pointers := make([]*domain.Order, 0, len(orders))
	for i := range orders {
		pointers = append(pointers, &orders[i])
	}
	if err := s.hydrate(ctx, pointers); err != nil {
		return nil, err
	}
	return orders, nil
}

// ListOrders implements the admin registry with filters, paging and counts.
func (s *Store) ListOrders(ctx context.Context, filter OrderFilter) (OrderList, error) {
	result := OrderList{Orders: []domain.Order{}, Counts: map[string]int{}}

	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	sortColumn := "o.created_at"
	if filter.Sort == "updated" {
		sortColumn = "o.updated_at"
	}

	where := []string{"1 = 1"}
	args := []any{}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("o.status = $%d", len(args)))
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		args = append(args, "%"+strings.ToLower(query)+"%")
		where = append(where,
			fmt.Sprintf(`(lower(o.number) like $%d or lower(o.topics) like $%d
				or lower(o.contact) like $%d or lower(o.contact_name) like $%d
				or lower(o.student_name) like $%d)`, len(args), len(args), len(args), len(args), len(args)))
	}
	whereSQL := strings.Join(where, " and ")

	if err := s.pool.QueryRow(ctx,
		`select count(*) from orders o where `+whereSQL, args...).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("store: count orders: %w", err)
	}

	// payment_claimed is always pinned to the top: that is the operator's queue.
	sql := `select ` + orderCols("o.") + ` from orders o where ` + whereSQL +
		` order by (o.status = 'payment_claimed') desc, ` + sortColumn + ` desc
		  limit $` + itoa(len(args)+1) + ` offset $` + itoa(len(args)+2)
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return result, fmt.Errorf("store: list orders: %w", err)
	}
	defer rows.Close()

	orders, err := collectOrders(rows)
	if err != nil {
		return result, err
	}
	pointers := make([]*domain.Order, 0, len(orders))
	for i := range orders {
		pointers = append(pointers, &orders[i])
	}
	if err := s.hydrate(ctx, pointers); err != nil {
		return result, err
	}
	result.Orders = orders

	counts, err := s.CountByStatus(ctx)
	if err != nil {
		return result, err
	}
	result.Counts = counts
	return result, nil
}

// CountByStatus returns orders per status (KPI cards, bot /stats).
func (s *Store) CountByStatus(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{}
	rows, err := s.pool.Query(ctx, `select status, count(*) from orders group by status`)
	if err != nil {
		return counts, fmt.Errorf("store: counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return counts, fmt.Errorf("store: scan count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// ListOrdersForChat returns the orders a Telegram chat has linked, newest
// first, up to limit. Zero (or an out-of-range value) falls back to 20: the
// bot lists a handful, the Mini App asks for more.
func (s *Store) ListOrdersForChat(ctx context.Context, chatID int64, limit int) ([]domain.Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`select `+orderCols("o.")+` from orders o where o.telegram_chat_id = $1
		  order by o.created_at desc limit $2`, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: orders for chat: %w", err)
	}
	defer rows.Close()
	orders, err := collectOrders(rows)
	if err != nil {
		return nil, err
	}
	pointers := make([]*domain.Order, 0, len(orders))
	for i := range orders {
		pointers = append(pointers, &orders[i])
	}
	if err := s.hydrate(ctx, pointers); err != nil {
		return nil, err
	}
	return orders, nil
}

// ListOrdersByStatus returns up to limit orders in the given statuses,
// newest first (used by the operator commands in the bot).
func (s *Store) ListOrdersByStatus(ctx context.Context, statuses []string, limit int) ([]domain.Order, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx,
		`select `+orderCols("o.")+` from orders o where o.status = any($1::text[])
		  order by o.created_at desc limit $2`, statuses, limit)
	if err != nil {
		return nil, fmt.Errorf("store: orders by status: %w", err)
	}
	defer rows.Close()
	orders, err := collectOrders(rows)
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// LinkOrderChat binds a Telegram chat to an order so status pushes reach it.
func (s *Store) LinkOrderChat(ctx context.Context, number string, chatID int64, contact string) error {
	tag, err := s.pool.Exec(ctx,
		`update orders set telegram_chat_id = $2,
		        contact = case when contact = '' then $3 else contact end,
		        contact_type = 'telegram',
		        updated_at = now()
		  where number = $1`, number, chatID, contact)
	if err != nil {
		return fmt.Errorf("store: link chat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// collectOrders turns rows into orders.
func collectOrders(rows pgx.Rows) ([]domain.Order, error) {
	orders := []domain.Order{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan order: %w", err)
		}
		orders = append(orders, *order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: order rows: %w", err)
	}
	return orders, nil
}

// hydrate fills attachments, timeline entries and the result file for a batch
// of orders with three extra queries.
func (s *Store) hydrate(ctx context.Context, orders []*domain.Order) error {
	if len(orders) == 0 {
		return nil
	}
	numbers := make([]string, 0, len(orders))
	for _, order := range orders {
		numbers = append(numbers, order.Number)
	}

	attachments := map[string][]domain.FileMeta{}
	rows, err := s.pool.Query(ctx,
		`select order_number, file_name, size_bytes, mime_type
		   from order_files
		  where kind = 'client_material' and order_number = any($1::text[])
		  order by id`, numbers)
	if err != nil {
		return fmt.Errorf("store: attachments: %w", err)
	}
	for rows.Next() {
		var number, name, mime string
		var size int64
		if err := rows.Scan(&number, &name, &size, &mime); err != nil {
			rows.Close()
			return fmt.Errorf("store: scan attachment: %w", err)
		}
		attachments[number] = append(attachments[number], domain.FileMeta{Name: name, Size: size, Type: mime})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: attachment rows: %w", err)
	}

	events := map[string][]domain.OrderEvent{}
	rows, err = s.pool.Query(ctx,
		`select order_number, actor, text, at
		   from order_events
		  where order_number = any($1::text[])
		  order by at desc, id desc`, numbers)
	if err != nil {
		return fmt.Errorf("store: events: %w", err)
	}
	for rows.Next() {
		var number, actor, text string
		var at time.Time
		if err := rows.Scan(&number, &actor, &text, &at); err != nil {
			rows.Close()
			return fmt.Errorf("store: scan event: %w", err)
		}
		events[number] = append(events[number], domain.OrderEvent{
			At: at.UTC().Format(time.RFC3339), Actor: actor, Text: text,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: event rows: %w", err)
	}

	results, err := s.ResultFiles(ctx, numbers)
	if err != nil {
		return err
	}

	for _, order := range orders {
		if list, ok := attachments[order.Number]; ok {
			order.Attachments = list
		}
		if list, ok := events[order.Number]; ok {
			order.Events = list
		}
		if files, ok := results[order.Number]; ok && len(files) > 0 {
			order.ResultFiles = make([]domain.ResultFile, 0, len(files))
			for _, file := range files {
				order.ResultFiles = append(order.ResultFiles, domain.ResultFile{
					ID: file.ID, Name: file.FileName, Size: file.SizeBytes, Type: file.MimeType,
					ExpiresAt: utcPtr(file.ExpiresAt),
				})
			}
			order.ResultFile = &order.ResultFiles[0]
		}
	}
	return nil
}

// ResultFiles returns the ready documents of the given orders: the files of the
// newest generation run (an order with several works has one file per work).
func (s *Store) ResultFiles(ctx context.Context, numbers []string) (map[string][]OrderFileRow, error) {
	out := map[string][]OrderFileRow{}
	if len(numbers) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx,
		`select f.id, f.order_number, f.kind, f.file_name, f.size_bytes, f.mime_type, f.storage_key,
		        f.attempt, f.created_at, f.expires_at
		   from order_files f
		  where f.kind = 'result' and f.order_number = any($1::text[])
		    and f.attempt = (select max(x.attempt) from order_files x
		                       where x.order_number = f.order_number and x.kind = 'result')
		  order by f.order_number, f.id`, numbers)
	if err != nil {
		return out, fmt.Errorf("store: result files: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var file OrderFileRow
		if err := rows.Scan(&file.ID, &file.Number, &file.Kind, &file.FileName, &file.SizeBytes,
			&file.MimeType, &file.StorageKey, &file.Attempt, &file.CreatedAt, &file.ExpiresAt); err != nil {
			return out, fmt.Errorf("store: scan result file: %w", err)
		}
		out[file.Number] = append(out[file.Number], file)
	}
	return out, rows.Err()
}

// GetResultFile returns the newest ready document of one order.
func (s *Store) GetResultFile(ctx context.Context, number string) (OrderFileRow, error) {
	var file OrderFileRow
	err := s.pool.QueryRow(ctx,
		`select id, order_number, kind, file_name, size_bytes, mime_type, storage_key, attempt, created_at, expires_at
		   from order_files
		  where order_number = $1 and kind = 'result'
		  order by created_at desc limit 1`, number,
	).Scan(&file.ID, &file.Number, &file.Kind, &file.FileName, &file.SizeBytes,
		&file.MimeType, &file.StorageKey, &file.Attempt, &file.CreatedAt, &file.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return file, ErrNotFound
	}
	if err != nil {
		return file, fmt.Errorf("store: get result file: %w", err)
	}
	return file, nil
}

// ListResultFiles returns the documents of the newest generation run of one
// order (several works -> several files), oldest first.
func (s *Store) ListResultFiles(ctx context.Context, number string) ([]OrderFileRow, error) {
	rows, err := s.pool.Query(ctx,
		`select f.id, f.order_number, f.kind, f.file_name, f.size_bytes, f.mime_type, f.storage_key,
		        f.attempt, f.created_at, f.expires_at
		   from order_files f
		  where f.order_number = $1 and f.kind = 'result'
		    and f.attempt = (select max(x.attempt) from order_files x
		                       where x.order_number = f.order_number and x.kind = 'result')
		  order by f.id`, number)
	if err != nil {
		return nil, fmt.Errorf("store: list result files: %w", err)
	}
	defer rows.Close()
	files := []OrderFileRow{}
	for rows.Next() {
		var file OrderFileRow
		if err := rows.Scan(&file.ID, &file.Number, &file.Kind, &file.FileName, &file.SizeBytes,
			&file.MimeType, &file.StorageKey, &file.Attempt, &file.CreatedAt, &file.ExpiresAt); err != nil {
			return nil, fmt.Errorf("store: scan result file: %w", err)
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

// GetResultFileByID resolves one result document of an order (per-file links).
func (s *Store) GetResultFileByID(ctx context.Context, number string, id int64) (OrderFileRow, error) {
	var file OrderFileRow
	err := s.pool.QueryRow(ctx,
		`select id, order_number, kind, file_name, size_bytes, mime_type, storage_key, attempt, created_at, expires_at
		   from order_files
		  where order_number = $1 and kind = 'result' and id = $2`, number, id,
	).Scan(&file.ID, &file.Number, &file.Kind, &file.FileName, &file.SizeBytes,
		&file.MimeType, &file.StorageKey, &file.Attempt, &file.CreatedAt, &file.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return file, ErrNotFound
	}
	if err != nil {
		return file, fmt.Errorf("store: get result file by id: %w", err)
	}
	return file, nil
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}
