package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/auth"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/store"
)

// Client authorization through the bot:
//
//	POST /api/auth/telegram/start -> one-time code (5 minutes)
//	the client sends /login <code> to the bot
//	POST /api/auth/telegram/poll  -> client session for the chat
//
// The chat id is the client identity: orders created with the session are
// attached to Telegram automatically. When the same chat is bound to an
// operator, the poll also opens a panel session, so one bot login gives the
// operator both roles.

// ClientAuthStart is the response of POST /api/auth/telegram/start.
type ClientAuthStart struct {
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expiresAt"`
	BotURL      string    `json:"botUrl,omitempty"`
	DeepLink    string    `json:"deepLink,omitempty"`
	Instruction string    `json:"instruction"`
}

// OperatorPart is the panel session issued together with a client session when
// the Telegram chat belongs to an operator.
type OperatorPart struct {
	Session *domain.AdminSession `json:"session"`
	Token   string               `json:"token"`
}

// ClientAuthResult is the payload of a confirmed client login.
type ClientAuthResult struct {
	Session  *domain.ClientSession `json:"session,omitempty"`
	Token    string                `json:"token,omitempty"`
	Operator *OperatorPart         `json:"operator,omitempty"`
}

// StartClientLogin creates the one-time code the client sends to the bot.
func (s *Service) StartClientLogin(ctx context.Context) (ClientAuthStart, error) {
	code, err := auth.NumericCode(6)
	if err != nil {
		return ClientAuthStart{}, err
	}
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if err := s.Store.CreateLoginChallenge(ctx, code, "", store.ChallengeClient, expiresAt); err != nil {
		return ClientAuthStart{}, err
	}

	start := ClientAuthStart{
		Code:      code,
		ExpiresAt: expiresAt,
		Instruction: "Откройте бота и отправьте ему команду /login " + code +
			". Окно подтверждения - 5 минут.",
	}
	if s.Cfg.Bot.Username != "" {
		start.BotURL = "https://t.me/" + s.Cfg.Bot.Username
		start.DeepLink = "https://t.me/" + s.Cfg.Bot.Username + "?start=login_" + code
	}
	return start, nil
}

// PollClientLogin exchanges a confirmed code for a client session. pending=true
// means the bot has not confirmed it yet.
func (s *Service) PollClientLogin(ctx context.Context, code, ip, userAgent string) (ClientAuthResult, bool, error) {
	challenge, err := s.Store.GetLoginChallenge(ctx, strings.TrimSpace(code))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ClientAuthResult{}, false, NotFound("Код не найден или истёк, запросите новый")
		}
		return ClientAuthResult{}, false, err
	}
	if challenge.Kind != store.ChallengeClient {
		return ClientAuthResult{}, false, NotFound("Этот код выдан для входа оператора")
	}
	if challenge.UsedAt != nil {
		return ClientAuthResult{}, false, NotFound("Код уже использован, запросите новый")
	}
	if challenge.ConfirmedAt == nil || challenge.ChatID == nil {
		return ClientAuthResult{}, true, nil // still waiting for the bot
	}

	if err := s.Store.ConsumeLoginChallenge(ctx, challenge.Code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ClientAuthResult{}, false, NotFound("Код не найден или истёк, запросите новый")
		}
		return ClientAuthResult{}, false, err
	}

	chatID := *challenge.ChatID
	now := time.Now().UTC()

	ttl := s.Cfg.ClientSessionTTL
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour // a hand-built config must not expire sessions instantly
	}

	token, err := auth.SessionToken()
	if err != nil {
		return ClientAuthResult{}, false, err
	}
	if err := s.Store.CreateClientSession(ctx, auth.HashToken(s.Cfg.TokenPepper, token),
		chatID, challenge.ChatUsername, now.Add(ttl), ip, userAgent); err != nil {
		return ClientAuthResult{}, false, err
	}

	result := ClientAuthResult{
		Token: token,
		Session: &domain.ClientSession{
			ChatID:   chatID,
			Username: challenge.ChatUsername,
			Since:    now,
		},
	}

	// The same chat may belong to an operator: then the panel opens too.
	if operator, err := s.Store.OperatorByChatID(ctx, chatID); err == nil {
		adminToken, err := auth.SessionToken()
		if err != nil {
			return ClientAuthResult{}, false, err
		}
		if err := s.Store.CreateSession(ctx, auth.HashToken(s.Cfg.TokenPepper, adminToken),
			operator.ID, now.Add(s.Cfg.SessionTTL), ip, userAgent); err != nil {
			return ClientAuthResult{}, false, err
		}
		name := operator.DisplayName
		if name == "" {
			name = operator.ID
		}
		result.Operator = &OperatorPart{
			Token: adminToken,
			Session: &domain.AdminSession{
				OperatorID: operator.ID,
				Name:       name,
				Since:      now,
			},
		}
	}
	return result, false, nil
}

// ClientSession resolves a client bearer token (used by the API middleware).
func (s *Service) ClientSession(ctx context.Context, token string) (store.ClientSession, error) {
	if token == "" {
		return store.ClientSession{}, Unauthorized("Войдите через Telegram-бота, чтобы продолжить")
	}
	session, err := s.Store.GetClientSession(ctx, auth.HashToken(s.Cfg.TokenPepper, token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.ClientSession{}, Unauthorized("Сессия истекла, войдите через Telegram-бота заново")
		}
		return store.ClientSession{}, err
	}
	return session, nil
}

// ClientLogout closes the client session of this browser.
func (s *Service) ClientLogout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.Store.DeleteClientSession(ctx, auth.HashToken(s.Cfg.TokenPepper, token))
}

// ClientProfile is the public shape of a client session.
func ClientProfile(session store.ClientSession) domain.ClientSession {
	return domain.ClientSession{
		ChatID:   session.ChatID,
		Username: session.Username,
		Since:    session.CreatedAt,
	}
}

// ClientSessionFromInitData resolves the Telegram chat from a Mini App payload
// (the X-Telegram-Init-Data header). The Mini App keeps sending initData on
// every request, so no login code and no session row are needed: the HMAC
// signature over the payload is the proof, the bot token is the shared secret.
func (s *Service) ClientSessionFromInitData(raw string) (store.ClientSession, error) {
	if strings.TrimSpace(s.Cfg.Bot.Token) == "" {
		return store.ClientSession{}, Unauthorized("Mini App недоступен: Telegram-бот не настроен")
	}
	data, err := auth.ValidateInitData(raw, s.Cfg.Bot.Token, auth.DefaultInitDataTTL)
	if err != nil {
		if errors.Is(err, auth.ErrInitDataExpired) {
			return store.ClientSession{}, Unauthorized("Сеанс Telegram истёк, откройте приложение заново")
		}
		return store.ClientSession{}, Unauthorized("Не удалось проверить данные Telegram")
	}
	return store.ClientSession{
		ChatID:    data.User.ID,
		Username:  data.User.Username,
		CreatedAt: time.Now().UTC(),
	}, nil
}
