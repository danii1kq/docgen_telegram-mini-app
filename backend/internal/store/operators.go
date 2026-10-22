package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Operator is an admin-panel user (not part of the public API contract).
type Operator struct {
	ID             string
	PasswordHash   string
	DisplayName    string
	TelegramChatID *int64
	Active         bool
}

// SessionRow is a resolved admin session.
type SessionRow struct {
	OperatorID  string
	DisplayName string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// GetOperator loads one operator by handle (ErrNotFound otherwise).
func (s *Store) GetOperator(ctx context.Context, id string) (Operator, error) {
	var operator Operator
	err := s.pool.QueryRow(ctx,
		`select id, password_hash, display_name, telegram_chat_id, active
		   from operators where id = $1`, id,
	).Scan(&operator.ID, &operator.PasswordHash,
		&operator.DisplayName, &operator.TelegramChatID, &operator.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return operator, ErrNotFound
	}
	if err != nil {
		return operator, fmt.Errorf("store: get operator: %w", err)
	}
	return operator, nil
}

// OperatorByChatID resolves the operator bound to a Telegram chat. Several
// operators and several chats may be bound at the same time (operator_chats);
// the legacy single-column link is still honoured.
func (s *Store) OperatorByChatID(ctx context.Context, chatID int64) (Operator, error) {
	var operator Operator
	err := s.pool.QueryRow(ctx,
		`select o.id, o.password_hash, o.display_name, o.telegram_chat_id, o.active
		   from operators o
		  where o.active
		    and (o.telegram_chat_id = $1
		         or exists (select 1 from operator_chats c
		                     where c.chat_id = $1 and c.operator_id = o.id))
		  order by o.id
		  limit 1`, chatID,
	).Scan(&operator.ID, &operator.PasswordHash,
		&operator.DisplayName, &operator.TelegramChatID, &operator.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return operator, ErrNotFound
	}
	if err != nil {
		return operator, fmt.Errorf("store: operator by chat: %w", err)
	}
	return operator, nil
}

// OperatorChatIDs lists the chats that should receive operator notifications.
func (s *Store) OperatorChatIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx,
		`select distinct chat_id from (
		   select c.chat_id from operator_chats c
		     join operators o on o.id = c.operator_id
		    where o.active
		   union
		   select o.telegram_chat_id from operators o
		    where o.active and o.telegram_chat_id is not null
		 ) chats`)
	if err != nil {
		return nil, fmt.Errorf("store: operator chats: %w", err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan operator chat: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpsertOperator creates or replaces an operator (seed and `-operator` CLI).
func (s *Store) UpsertOperator(ctx context.Context, operator Operator) error {
	_, err := s.pool.Exec(ctx,
		`insert into operators (id, password_hash, display_name, active)
		 values ($1, $2, $3, $4)
		 on conflict (id) do update
		   set password_hash = excluded.password_hash,
		       display_name = excluded.display_name,
		       active = excluded.active`,
		operator.ID, operator.PasswordHash, operator.DisplayName, operator.Active)
	if err != nil {
		return fmt.Errorf("store: upsert operator: %w", err)
	}
	return nil
}

// SetOperatorPassword replaces only the password hash.
func (s *Store) SetOperatorPassword(ctx context.Context, id, passwordHash string) error {
	tag, err := s.pool.Exec(ctx,
		`update operators set password_hash = $2 where id = $1`, id, passwordHash)
	if err != nil {
		return fmt.Errorf("store: set operator password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// BindOperatorChat links a Telegram chat to an operator account. Any number of
// chats (and operators) may be bound at once: /bind never unlinks the chat of
// another operator. A nil chatID unlinks.
func (s *Store) BindOperatorChat(ctx context.Context, id string, chatID *int64) error {
	if chatID == nil {
		if _, err := s.pool.Exec(ctx,
			`update operators set telegram_chat_id = null where id = $1`, id); err != nil {
			return fmt.Errorf("store: unbind operator chat: %w", err)
		}
		return nil
	}

	var exists bool
	if err := s.pool.QueryRow(ctx,
		`select exists (select 1 from operators where id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("store: bind operator chat: %w", err)
	}
	if !exists {
		return ErrNotFound
	}

	if _, err := s.pool.Exec(ctx,
		`insert into operator_chats (chat_id, operator_id) values ($1, $2)
		 on conflict (chat_id) do update set operator_id = excluded.operator_id`,
		*chatID, id); err != nil {
		return fmt.Errorf("store: bind operator chat: %w", err)
	}
	// Keep the legacy column in step (older queries and tooling read it).
	if _, err := s.pool.Exec(ctx,
		`update operators set telegram_chat_id = coalesce(telegram_chat_id, $2) where id = $1`,
		id, *chatID); err != nil {
		return fmt.Errorf("store: bind operator chat: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// sessions
// ---------------------------------------------------------------------------

// CreateSession stores a new admin session (only the token hash is kept).
func (s *Store) CreateSession(ctx context.Context, tokenHash, operatorID string, expiresAt time.Time, ip, userAgent string) error {
	_, err := s.pool.Exec(ctx,
		`insert into admin_sessions (token_hash, operator_id, expires_at, ip, user_agent)
		 values ($1, $2, $3, $4, $5)`,
		tokenHash, operatorID, expiresAt, ip, userAgent)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// GetSession resolves a session token that is still valid.
func (s *Store) GetSession(ctx context.Context, tokenHash string) (SessionRow, error) {
	var row SessionRow
	err := s.pool.QueryRow(ctx,
		`select s.operator_id, o.display_name, s.created_at, s.expires_at
		   from admin_sessions s
		   join operators o on o.id = s.operator_id
		  where s.token_hash = $1 and s.expires_at > now() and o.active`,
		tokenHash,
	).Scan(&row.OperatorID, &row.DisplayName, &row.CreatedAt, &row.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("store: get session: %w", err)
	}
	row.CreatedAt = utc(row.CreatedAt)
	row.ExpiresAt = utc(row.ExpiresAt)

	// Refresh last_seen_at at most every 5 minutes (cheap write throttling).
	_, _ = s.pool.Exec(ctx,
		`update admin_sessions set last_seen_at = now()
		  where token_hash = $1 and last_seen_at < now() - interval '5 minutes'`, tokenHash)
	return row, nil
}

// DeleteSession logs the operator out.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.pool.Exec(ctx,
		`delete from admin_sessions where token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// PurgeExpiredSessions removes stale sessions, login attempts and challenges.
func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `delete from admin_sessions where expires_at < now()`); err != nil {
		return fmt.Errorf("store: purge sessions: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `delete from client_sessions where expires_at < now()`); err != nil {
		return fmt.Errorf("store: purge client sessions: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`delete from login_attempts where at < now() - interval '24 hours'`); err != nil {
		return fmt.Errorf("store: purge login attempts: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`delete from telegram_login_challenges where expires_at < now() - interval '1 hour'`); err != nil {
		return fmt.Errorf("store: purge challenges: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// login attempts (15-minute lockout)
// ---------------------------------------------------------------------------

// RecordFailedLogin notes a failed attempt for a key like "operator|ip".
func (s *Store) RecordFailedLogin(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx,
		`insert into login_attempts (login_key, successful) values ($1, false)`, key)
	if err != nil {
		return fmt.Errorf("store: record login attempt: %w", err)
	}
	return nil
}

// ClearLoginAttempts forgets the failures of a key after a successful login.
func (s *Store) ClearLoginAttempts(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `delete from login_attempts where login_key = $1`, key)
	if err != nil {
		return fmt.Errorf("store: clear login attempts: %w", err)
	}
	return nil
}

// CountFailedLogins counts failures inside the lockout window.
func (s *Store) CountFailedLogins(ctx context.Context, key string, window time.Duration) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`select count(*) from login_attempts
		  where login_key = $1 and successful = false and at > $2`,
		key, time.Now().UTC().Add(-window)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count login attempts: %w", err)
	}
	return count, nil
}
