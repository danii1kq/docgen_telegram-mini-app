package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ClientSession is a browser session opened through the Telegram bot. The chat
// id is the client's identity: orders created with this session are attached to
// Telegram automatically and receive the status notifications.
type ClientSession struct {
	TokenHash  string
	ChatID     int64
	Username   string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

// CreateClientSession stores a new client session (only the token hash is kept).
func (s *Store) CreateClientSession(ctx context.Context, tokenHash string, chatID int64, username string, expiresAt time.Time, ip, userAgent string) error {
	_, err := s.pool.Exec(ctx,
		`insert into client_sessions (token_hash, chat_id, username, expires_at, ip, user_agent)
		 values ($1, $2, $3, $4, $5, $6)`,
		tokenHash, chatID, username, expiresAt, ip, userAgent)
	if err != nil {
		return fmt.Errorf("store: create client session: %w", err)
	}
	return nil
}

// GetClientSession resolves a live client session by its token hash.
func (s *Store) GetClientSession(ctx context.Context, tokenHash string) (ClientSession, error) {
	var session ClientSession
	err := s.pool.QueryRow(ctx,
		`select token_hash, chat_id, username, created_at, expires_at, last_seen_at
		   from client_sessions
		  where token_hash = $1 and expires_at > now()`, tokenHash,
	).Scan(&session.TokenHash, &session.ChatID, &session.Username,
		&session.CreatedAt, &session.ExpiresAt, &session.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, ErrNotFound
	}
	if err != nil {
		return session, fmt.Errorf("store: get client session: %w", err)
	}
	session.CreatedAt = utc(session.CreatedAt)
	session.ExpiresAt = utc(session.ExpiresAt)
	session.LastSeenAt = utc(session.LastSeenAt)

	// Refresh last_seen_at at most every 5 minutes, like the admin sessions.
	_, _ = s.pool.Exec(ctx,
		`update client_sessions set last_seen_at = now()
		  where token_hash = $1 and last_seen_at < now() - interval '5 minutes'`, tokenHash)
	return session, nil
}

// DeleteClientSession logs the client out of this browser.
func (s *Store) DeleteClientSession(ctx context.Context, tokenHash string) error {
	if _, err := s.pool.Exec(ctx,
		`delete from client_sessions where token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("store: delete client session: %w", err)
	}
	return nil
}
