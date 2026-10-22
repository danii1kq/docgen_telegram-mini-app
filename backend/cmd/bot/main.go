// Command bot runs the Telegram bot and the generation worker without the
// HTTP API (useful when the API runs on another host and both share the
// database). The bot normally runs inside cmd/api instead.
//
//	go run ./cmd/bot
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/tgbot"
	"github.com/srunal/srunal-go/internal/worker"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}
	if !cfg.BotConfigured() {
		log.Error("telegram bot is not configured: set TELEGRAM_BOT_ENABLED=true and TELEGRAM_BOT_TOKEN in .env")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("database", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}

	svc := service.New(st, cfg, log)
	if err := svc.EnsureStorage(); err != nil {
		log.Error("storage", "error", err)
		os.Exit(1)
	}

	if cfg.Worker.Enabled {
		go worker.New(svc, log).Run(ctx)
	}

	bot := tgbot.New(svc, cfg, log)
	if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("bot stopped", "error", err)
		os.Exit(1)
	}
	log.Info("bot stopped")
}
