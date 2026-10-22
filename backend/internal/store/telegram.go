package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TelegramMessage is one queued outbound bot message.
type TelegramMessage struct {
	ID          int64
	ChatID      int64
	Text        string
	OrderNumber *string
	// Buttons is the inline keyboard (reply_markup JSON), nil when the message
	// has no buttons.
	Buttons []byte
	// DocumentPath/DocumentName turn the message into a sendDocument call: the
	// result file of a finished order is delivered as a chat attachment.
	DocumentPath string
	DocumentName string
	Attempts     int
}

// LoginChallenge is a Telegram login code created by a web page.
type LoginChallenge struct {
	Code         string
	Kind         string // operator | client
	OperatorID   string
	ChatID       *int64
	ChatUsername string
	ExpiresAt    time.Time
	ConfirmedAt  *time.Time
	UsedAt       *time.Time
}

// Challenge kinds.
const (
	ChallengeOperator = "operator"
	ChallengeClient   = "client"
)

// KVGet reads a small key/value entry (Telegram offset, bot state).
func (s *Store) KVGet(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.pool.QueryRow(ctx, `select value from app_kv where key = $1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: kv get: %w", err)
	}
	return value, true, nil
}

// KVSet writes a small key/value entry.
func (s *Store) KVSet(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx,
		`insert into app_kv (key, value, updated_at) values ($1, $2, now())
		 on conflict (key) do update set value = excluded.value, updated_at = now()`,
		key, value)
	if err != nil {
		return fmt.Errorf("store: kv set: %w", err)
	}
	return nil
}

// EnqueueTelegram adds an outbound message. The bot loop flushes the queue, so
// a disabled or temporarily broken bot never blocks an API request. buttons is
// an optional inline keyboard (reply_markup JSON).
func (s *Store) EnqueueTelegram(ctx context.Context, chatID int64, text, orderNumber string, buttons []byte) error {
	return s.EnqueueTelegramDocument(ctx, chatID, text, orderNumber, "", "", buttons)
}

// EnqueueTelegramDocument queues a message that carries a file: the bot loop
// sends it with sendDocument (caption = text) instead of sendMessage. An empty
// documentPath keeps the plain text behaviour.
func (s *Store) EnqueueTelegramDocument(ctx context.Context, chatID int64, text, orderNumber, documentPath, documentName string, buttons []byte) error {
	var number *string
	if orderNumber != "" {
		number = &orderNumber
	}
	var markup any
	if len(buttons) > 0 {
		markup = buttons
	}
	_, err := s.pool.Exec(ctx,
		`insert into telegram_outbox (chat_id, text, order_number, buttons, document_path, document_name)
		 values ($1, $2, $3, $4::jsonb, $5, $6)`,
		chatID, text, number, markup, documentPath, documentName)
	if err != nil {
		return fmt.Errorf("store: enqueue telegram: %w", err)
	}
	return nil
}

// PendingTelegram returns the next unsent messages (oldest first).
func (s *Store) PendingTelegram(ctx context.Context, limit int) ([]TelegramMessage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`select id, chat_id, text, order_number, buttons, document_path, document_name, attempts
		   from telegram_outbox
		  where sent_at is null and attempts < 5
		  order by id
		  limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending telegram: %w", err)
	}
	defer rows.Close()

	messages := []TelegramMessage{}
	for rows.Next() {
		var message TelegramMessage
		if err := rows.Scan(&message.ID, &message.ChatID, &message.Text,
			&message.OrderNumber, &message.Buttons, &message.DocumentPath,
			&message.DocumentName, &message.Attempts); err != nil {
			return nil, fmt.Errorf("store: scan telegram message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// MarkTelegramSent closes a message.
func (s *Store) MarkTelegramSent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`update telegram_outbox set sent_at = now(), attempts = attempts + 1 where id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: mark telegram sent: %w", err)
	}
	return nil
}

// MarkTelegramFailed records the error so the message can be retried later.
func (s *Store) MarkTelegramFailed(ctx context.Context, id int64, errText string) error {
	_, err := s.pool.Exec(ctx,
		`update telegram_outbox set attempts = attempts + 1, last_error = $2 where id = $1`,
		id, errText)
	if err != nil {
		return fmt.Errorf("store: mark telegram failed: %w", err)
	}
	return nil
}

// DropTelegramForChat fails every pending message of a chat (bot blocked).
func (s *Store) DropTelegramForChat(ctx context.Context, chatID int64) error {
	_, err := s.pool.Exec(ctx,
		`update telegram_outbox set sent_at = now(), last_error = 'chat not reachable'
		  where chat_id = $1 and sent_at is null`, chatID)
	if err != nil {
		return fmt.Errorf("store: drop telegram for chat: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Telegram login challenges (admin login through the bot)
// ---------------------------------------------------------------------------

// CreateLoginChallenge stores a one-time code for the login pages. kind is
// `operator` (panel session) or `client` (chat becomes the client identity);
// operatorID may be empty when the chat binding decides.
func (s *Store) CreateLoginChallenge(ctx context.Context, code, operatorID, kind string, expiresAt time.Time) error {
	if kind == "" {
		kind = ChallengeOperator
	}
	_, err := s.pool.Exec(ctx,
		`insert into telegram_login_challenges (code, operator_id, kind, expires_at)
		 values ($1, $2, $3, $4)`,
		code, operatorID, kind, expiresAt)
	if err != nil {
		return classify(fmt.Errorf("store: create challenge: %w", err))
	}
	return nil
}

// ConfirmLoginChallenge marks a code confirmed by a Telegram chat.
func (s *Store) ConfirmLoginChallenge(ctx context.Context, code string, chatID int64, username string) error {
	tag, err := s.pool.Exec(ctx,
		`update telegram_login_challenges
		    set confirmed_at = now(), chat_id = $2, chat_username = $3
		  where code = $1 and used_at is null and expires_at > now()`,
		code, chatID, username)
	if err != nil {
		return fmt.Errorf("store: confirm challenge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetLoginChallenge reads one challenge (ErrNotFound when unknown/expired).
func (s *Store) GetLoginChallenge(ctx context.Context, code string) (LoginChallenge, error) {
	var challenge LoginChallenge
	err := s.pool.QueryRow(ctx,
		`select code, kind, operator_id, chat_id, chat_username, expires_at, confirmed_at, used_at
		   from telegram_login_challenges
		  where code = $1 and expires_at > now()`, code,
	).Scan(&challenge.Code, &challenge.Kind, &challenge.OperatorID, &challenge.ChatID,
		&challenge.ChatUsername, &challenge.ExpiresAt, &challenge.ConfirmedAt, &challenge.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return challenge, ErrNotFound
	}
	if err != nil {
		return challenge, fmt.Errorf("store: get challenge: %w", err)
	}
	challenge.ExpiresAt = utc(challenge.ExpiresAt)
	challenge.ConfirmedAt = utcPtr(challenge.ConfirmedAt)
	challenge.UsedAt = utcPtr(challenge.UsedAt)
	return challenge, nil
}

// ConsumeLoginChallenge marks a confirmed code as used (single use).
func (s *Store) ConsumeLoginChallenge(ctx context.Context, code string) error {
	tag, err := s.pool.Exec(ctx,
		`update telegram_login_challenges set used_at = now()
		  where code = $1 and used_at is null and confirmed_at is not null and expires_at > now()`,
		code)
	if err != nil {
		return fmt.Errorf("store: consume challenge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
