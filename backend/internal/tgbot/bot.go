package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/service"
)

// kvOffsetKey stores the getUpdates offset so restarts do not replay updates.
const kvOffsetKey = "telegram_offset"

// messageSender is the slice of the Bot API the command handlers need (a seam
// for tests).
type messageSender interface {
	sendMessage(ctx context.Context, chatID int64, text string, markup json.RawMessage) error
	answerCallbackQuery(ctx context.Context, id, text string) error
	editMessageText(ctx context.Context, chatID, messageID int64, text string) error
}

// Bot runs the long polling loop and flushes the notification outbox.
type Bot struct {
	svc    *service.Service
	cfg    *config.Config
	log    *slog.Logger
	client *client
	// sender overrides the HTTP client in tests.
	sender messageSender

	offset int64
}

// New builds the bot.
func New(svc *service.Service, cfg *config.Config, log *slog.Logger) *Bot {
	return &Bot{
		svc:    svc,
		cfg:    cfg,
		log:    log,
		client: newClient(cfg.Bot.Token),
	}
}

// Run starts the polling loop and the outbox flusher; it blocks until the
// context is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	me, err := b.client.getMe(ctx)
	if err != nil {
		return err
	}
	b.log.Info("telegram bot connected", "username", "@"+me.Username, "id", me.ID)

	if raw, ok, err := b.svc.Store.KVGet(ctx, kvOffsetKey); err == nil && ok {
		if parsed, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			b.offset = parsed
		}
	}

	// The notification outbox is flushed in parallel with the long poll.
	go b.outboxLoop(ctx)

	b.log.Info("telegram bot polling started", "timeout", b.cfg.Bot.PollTimeout.String())
	for {
		if ctx.Err() != nil {
			b.log.Info("telegram bot stopped")
			return ctx.Err()
		}
		if err := b.pollOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			var apiErr *apiError
			if errors.As(err, &apiErr) && apiErr.Code == 429 && apiErr.RetryAfter > 0 {
				b.log.Warn("telegram rate limited", "retry_after", apiErr.RetryAfter)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(apiErr.RetryAfter) * time.Second):
				}
				continue
			}
			// Network hiccup: back off briefly and retry, never crash the API.
			b.log.Error("telegram poll failed", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// pollOnce fetches and handles one batch of updates.
func (b *Bot) pollOnce(ctx context.Context) error {
	timeout := b.cfg.Bot.PollTimeout
	updates, err := b.client.getUpdates(ctx, b.offset, timeout)
	if err != nil {
		return err
	}
	for _, update := range updates {
		b.offset = update.UpdateID + 1
		switch {
		case update.CallbackQuery != nil:
			b.handleCallback(ctx, *update.CallbackQuery)
		case update.Message != nil:
			if strings.TrimSpace(update.Message.Text) == "" {
				continue
			}
			b.handleMessage(ctx, *update.Message)
		}
	}
	if len(updates) > 0 {
		if err := b.svc.Store.KVSet(ctx, kvOffsetKey, strconv.FormatInt(b.offset, 10)); err != nil {
			b.log.Error("telegram offset save failed", "error", err)
		}
	}
	return nil
}

// outboxLoop sends queued notifications every few seconds.
func (b *Bot) outboxLoop(ctx context.Context) {
	interval := 3 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.flushOutbox(ctx); err != nil {
				b.log.Error("telegram outbox flush failed", "error", err)
			}
		}
	}
}

// flushOutbox delivers pending notifications, retrying transient errors.
func (b *Bot) flushOutbox(ctx context.Context) error {
	messages, err := b.svc.Store.PendingTelegram(ctx, 20)
	if err != nil {
		return err
	}
	for _, message := range messages {
		sendCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		var err error
		if message.DocumentPath != "" {
			err = b.client.sendDocument(sendCtx, message.ChatID, message.DocumentPath,
				message.DocumentName, message.Text, json.RawMessage(message.Buttons))
			if err != nil && errors.Is(err, os.ErrNotExist) {
				// The file was cleaned up (result TTL): the caption still tells
				// the client what happened, so deliver it as a plain message.
				b.log.Warn("telegram document missing, sending text", "id", message.ID, "path", message.DocumentPath)
				err = b.client.sendMessage(sendCtx, message.ChatID, message.Text, json.RawMessage(message.Buttons))
			}
		} else {
			err = b.client.sendMessage(sendCtx, message.ChatID, message.Text, json.RawMessage(message.Buttons))
		}
		cancel()

		if err == nil {
			if markErr := b.svc.Store.MarkTelegramSent(ctx, message.ID); markErr != nil {
				b.log.Error("telegram mark sent failed", "id", message.ID, "error", markErr)
			}
			continue
		}

		var apiErr *apiError
		if errors.As(err, &apiErr) {
			// 403: the client blocked the bot; 400: bad chat id. Give up on
			// this message instead of retrying forever.
			if apiErr.Code == 403 || apiErr.Code == 400 {
				b.log.Warn("telegram message dropped", "id", message.ID, "error", apiErr.Error())
				if markErr := b.svc.Store.MarkTelegramFailed(ctx, message.ID, apiErr.Description); markErr != nil {
					b.log.Error("telegram mark failed", "error", markErr)
				}
				if apiErr.Code == 403 {
					_ = b.svc.Store.DropTelegramForChat(ctx, message.ChatID)
				}
				continue
			}
			if apiErr.Code == 429 && apiErr.RetryAfter > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(apiErr.RetryAfter) * time.Second):
				}
			}
		}
		if markErr := b.svc.Store.MarkTelegramFailed(ctx, message.ID, err.Error()); markErr != nil {
			b.log.Error("telegram mark failed", "error", markErr)
		}
		b.log.Warn("telegram send failed", "id", message.ID, "attempts", message.Attempts+1, "error", err)
	}
	return nil
}

// Send replies immediately inside the polling loop.
func (b *Bot) send(ctx context.Context, chatID int64, text string) {
	sender := b.sender
	if sender == nil {
		sender = b.client
	}
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := sender.sendMessage(sendCtx, chatID, text, nil); err != nil {
		b.log.Error("telegram reply failed", "chat_id", chatID, "error", err)
	}
}
