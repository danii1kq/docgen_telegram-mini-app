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

// LockoutMessage is shown for a locked operator (README section 5.6).
const LockoutMessage = "Слишком много попыток. Попробуйте через 15 минут."

// WrongCredentials is deliberately identical for an unknown handle and a bad
// password, so the login endpoint cannot be used to enumerate operators.
const WrongCredentials = "Неверный логин или пароль"

// BindWrongCredentials answers /bind in the bot. The handle is typed by the
// operator here, so the message points at the expected format (the panel login,
// e.g. @operator_almaty) instead of leaving them guessing.
const BindWrongCredentials = "Неверный логин или пароль. Логин оператора - как в панели, например @operator_almaty"

// LoginInput is the technical password login. The operator panel itself signs
// in through the Telegram bot (POST /api/admin/auth/telegram/*); this endpoint
// stays for tooling and break-glass access.
type LoginInput struct {
	OperatorID string `json:"operatorId"`
	Password   string `json:"password"`
	Code       string `json:"code"`
}

// LoginResult carries the panel session.
type LoginResult struct {
	Session *domain.AdminSession `json:"session,omitempty"`
	Token   string               `json:"token,omitempty"`
}

// AdminLogin verifies the password and opens a server-side session.
func (s *Service) AdminLogin(ctx context.Context, input LoginInput, ip, userAgent string) (LoginResult, error) {
	operatorID := normalizeOperatorID(input.OperatorID)
	password := input.Password
	if password == "" {
		password = input.Code // single-field clients put the password in `code`
	}

	lockKey := operatorID + "|" + ip
	failures, err := s.Store.CountFailedLogins(ctx, lockKey, s.Cfg.LoginLockout)
	if err != nil {
		return LoginResult{}, err
	}
	if failures >= s.Cfg.LoginMaxAttempts {
		return LoginResult{}, RateLimited(int(s.Cfg.LoginLockout.Seconds()), LockoutMessage)
	}

	operator, err := s.Store.GetOperator(ctx, operatorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = s.Store.RecordFailedLogin(ctx, lockKey)
			return LoginResult{}, Unauthorized(WrongCredentials)
		}
		return LoginResult{}, err
	}
	if !operator.Active || !auth.VerifyPassword(operator.PasswordHash, password) {
		_ = s.Store.RecordFailedLogin(ctx, lockKey)
		return LoginResult{}, Unauthorized(WrongCredentials)
	}

	return s.openSession(ctx, operator, lockKey, ip, userAgent)
}

// openSession creates the session row and the response.
func (s *Service) openSession(ctx context.Context, operator store.Operator, lockKey, ip, userAgent string) (LoginResult, error) {
	token, err := auth.SessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	expiresAt := time.Now().UTC().Add(s.Cfg.SessionTTL)
	if err := s.Store.CreateSession(ctx, auth.HashToken(s.Cfg.TokenPepper, token),
		operator.ID, expiresAt, ip, userAgent); err != nil {
		return LoginResult{}, err
	}
	_ = s.Store.ClearLoginAttempts(ctx, lockKey)

	name := operator.DisplayName
	if name == "" {
		name = operator.ID
	}
	return LoginResult{
		Token: token,
		Session: &domain.AdminSession{
			OperatorID: operator.ID,
			Name:       name,
			Since:      time.Now().UTC(),
		},
	}, nil
}

// AdminSession resolves a bearer token (used by the auth middleware).
func (s *Service) AdminSession(ctx context.Context, token string) (store.SessionRow, error) {
	if strings.TrimSpace(token) == "" {
		return store.SessionRow{}, Unauthorized("Требуется вход в панель")
	}
	row, err := s.Store.GetSession(ctx, auth.HashToken(s.Cfg.TokenPepper, token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.SessionRow{}, Unauthorized("Сессия истекла, войдите заново")
		}
		return store.SessionRow{}, err
	}
	return row, nil
}

// AdminSessionFromInitData opens the operator panel from the Mini App: the
// Telegram chat in the validated initData payload must be bound to an operator
// (/bind, TELEGRAM_ADMIN_CHAT_IDS or the seeded chat id). No password and no
// login code - the same trust model as the client side, with an operator check
// on top.
func (s *Service) AdminSessionFromInitData(ctx context.Context, raw string) (store.SessionRow, error) {
	if strings.TrimSpace(s.Cfg.Bot.Token) == "" {
		return store.SessionRow{}, Unauthorized("Mini App недоступен: Telegram-бот не настроен")
	}
	data, err := auth.ValidateInitData(raw, s.Cfg.Bot.Token, auth.DefaultInitDataTTL)
	if err != nil {
		if errors.Is(err, auth.ErrInitDataExpired) {
			return store.SessionRow{}, Unauthorized("Сеанс Telegram истёк, откройте приложение заново")
		}
		return store.SessionRow{}, Unauthorized("Не удалось проверить данные Telegram")
	}

	operator, err := s.Store.OperatorByChatID(ctx, data.User.ID)
	if err != nil {
		// Chats listed in TELEGRAM_ADMIN_CHAT_IDS are trusted without a row in
		// the operators table - the bot treats them exactly the same way.
		if s.isTrustedAdminChat(data.User.ID) {
			now := time.Now().UTC()
			return store.SessionRow{
				OperatorID:  telegramAdminOperatorID,
				DisplayName: "Telegram admin",
				CreatedAt:   now,
				ExpiresAt:   now.Add(s.Cfg.SessionTTL),
			}, nil
		}
		if errors.Is(err, store.ErrNotFound) {
			return store.SessionRow{}, Forbidden("Панель доступна только операторам")
		}
		return store.SessionRow{}, err
	}

	name := operator.DisplayName
	if name == "" {
		name = operator.ID
	}
	now := time.Now().UTC()
	return store.SessionRow{
		OperatorID:  operator.ID,
		DisplayName: name,
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.Cfg.SessionTTL),
	}, nil
}

// telegramAdminOperatorID stands in for chats listed in TELEGRAM_ADMIN_CHAT_IDS
// (they have no operators row).
const telegramAdminOperatorID = "@telegram_admin"

// isTrustedAdminChat reports whether the chat is configured as an operator chat.
func (s *Service) isTrustedAdminChat(chatID int64) bool {
	for _, admin := range s.Cfg.Bot.AdminChatIDs {
		if admin == chatID {
			return true
		}
	}
	return false
}

// OperatorForChat resolves the operator bound to a chat, if any (used by
// /api/auth/me so the Mini App knows whether to show the panel entry).
func (s *Service) OperatorForChat(ctx context.Context, chatID int64) (string, string, bool) {
	operator, err := s.Store.OperatorByChatID(ctx, chatID)
	if err == nil {
		name := operator.DisplayName
		if name == "" {
			name = operator.ID
		}
		return operator.ID, name, true
	}
	if s.isTrustedAdminChat(chatID) {
		return telegramAdminOperatorID, "Telegram admin", true
	}
	return "", "", false
}

// AdminLogout closes the session.
func (s *Service) AdminLogout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.Store.DeleteSession(ctx, auth.HashToken(s.Cfg.TokenPepper, token))
}

// ---------------------------------------------------------------------------
// Telegram login for the operator panel
// ---------------------------------------------------------------------------

// TelegramLoginStart is returned by POST /api/admin/auth/telegram/start.
type TelegramLoginStart struct {
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expiresAt"`
	BotURL      string    `json:"botUrl,omitempty"`
	DeepLink    string    `json:"deepLink,omitempty"`
	Instruction string    `json:"instruction"`
}

// StartTelegramLogin creates a one-time code the operator sends to the bot.
// The operator id is optional: an unbound chat may confirm any pending code,
// and the operator is resolved from the chat when the bot sees /login.
func (s *Service) StartTelegramLogin(ctx context.Context, operatorID string) (TelegramLoginStart, error) {
	code, err := auth.NumericCode(6)
	if err != nil {
		return TelegramLoginStart{}, err
	}
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if err := s.Store.CreateLoginChallenge(ctx, code, normalizeOperatorID(operatorID),
		store.ChallengeOperator, expiresAt); err != nil {
		return TelegramLoginStart{}, err
	}

	start := TelegramLoginStart{
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

// PollTelegramLogin is POST /api/admin/auth/telegram/poll. It returns
// pending=true while the operator has not confirmed the code in the bot.
func (s *Service) PollTelegramLogin(ctx context.Context, code, ip, userAgent string) (LoginResult, bool, error) {
	challenge, err := s.Store.GetLoginChallenge(ctx, strings.TrimSpace(code))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return LoginResult{}, false, NotFound("Код не найден или истёк, запросите новый")
		}
		return LoginResult{}, false, err
	}
	if challenge.Kind != store.ChallengeOperator {
		return LoginResult{}, false, NotFound("Этот код выдан для входа клиента")
	}
	if challenge.UsedAt != nil {
		// The code was already exchanged for a session: replaying it must not
		// open another one (and the page should stop polling).
		return LoginResult{}, false, NotFound("Код уже использован, запросите новый")
	}
	if challenge.ConfirmedAt == nil {
		return LoginResult{}, true, nil // still waiting for the bot
	}

	operatorID := challenge.OperatorID
	if challenge.ChatID != nil {
		if operator, err := s.Store.OperatorByChatID(ctx, *challenge.ChatID); err == nil {
			operatorID = operator.ID
		} else if operatorID == "" {
			return LoginResult{}, false, Unauthorized("Этот Telegram-чат не привязан к оператору")
		}
	}
	if operatorID == "" {
		return LoginResult{}, false, Unauthorized("Этот Telegram-чат не привязан к оператору")
	}

	operator, err := s.Store.GetOperator(ctx, operatorID)
	if err != nil || !operator.Active {
		return LoginResult{}, false, Unauthorized("Оператор не найден или отключён")
	}
	if err := s.Store.ConsumeLoginChallenge(ctx, challenge.Code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return LoginResult{}, false, NotFound("Код не найден или истёк, запросите новый")
		}
		return LoginResult{}, false, err
	}

	result, err := s.openSession(ctx, operator, operatorID+"|"+ip, ip, userAgent)
	if err != nil {
		return LoginResult{}, false, err
	}
	return result, false, nil
}

// BindOperatorChat verifies the password of an operator and links a chat.
// Called by the bot for `/bind <operator_id> <password>`.
func (s *Service) BindOperatorChat(ctx context.Context, operatorID, password string, chatID int64) error {
	handle := normalizeOperatorID(operatorID)
	operator, err := s.Store.GetOperator(ctx, handle)
	if err != nil || !operator.Active || !auth.VerifyPassword(operator.PasswordHash, password) {
		return Unauthorized(BindWrongCredentials)
	}
	return s.Store.BindOperatorChat(ctx, handle, &chatID)
}

func normalizeOperatorID(value string) string {
	handle := strings.TrimSpace(value)
	if handle != "" && !strings.HasPrefix(handle, "@") {
		handle = "@" + handle
	}
	return handle
}
