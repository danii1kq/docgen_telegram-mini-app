// Command api runs the SruNal KZ backend: HTTP API, generation worker and -
// when TELEGRAM_BOT_TOKEN is set - the Telegram bot in the same process.
//
//	go run ./cmd/api          # migrate + seed + serve on :8080
//	go run ./cmd/api -migrate # only apply migrations and exit
//	go run ./cmd/api -seed    # force-reimport the demo data into an empty DB
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/srunal/srunal-go/internal/api"
	"github.com/srunal/srunal-go/internal/auth"
	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/seed"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/tgbot"
	"github.com/srunal/srunal-go/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	migrateOnly := flag.Bool("migrate", false, "apply migrations and exit")
	forceSeed := flag.Bool("seed", false, "import demo data when the database is empty")
	operator := flag.String("operator", "", "operator handle for -password, e.g. @operator_almaty")
	password := flag.String("password", "", "set/replace the operator password and exit")
	operatorName := flag.String("operator-name", "", "display name when creating a new operator")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		return err
	}
	if *migrateOnly {
		log.Info("migrations applied")
		return nil
	}

	// Password rotation: -operator @handle -password 'secret'
	if *password != "" {
		if *operator == "" {
			return errors.New("use -operator @handle together with -password")
		}
		return setOperatorPassword(ctx, st, *operator, *password, *operatorName, log)
	}

	svc := service.New(st, cfg, log)
	if err := svc.EnsureStorage(); err != nil {
		return err
	}

	// The operator panel prices the generation spend in tenge as well: keep the
	// USD -> KZT rate fresh (НБ РК, public fallback) for the whole process.
	svc.StartUSDRateRefresher(ctx)

	if err := seedIfNeeded(ctx, st, cfg, svc, log, *forceSeed); err != nil {
		return err
	}

	server := api.New(svc, cfg, log)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	// Background loops: generation queue, session housekeeping, Telegram bot.
	var background []func()
	if cfg.Worker.Enabled {
		queue := worker.New(svc, log)
		background = append(background, func() {
			go func() {
				if cfg.Worker.QueuePauseOnStart {
					if err := st.SetQueuePaused(ctx, true); err != nil {
						log.Error("worker: initial pause failed", "error", err)
					}
				}
				queue.Run(ctx)
			}()
		})
	} else {
		log.Warn("generation worker disabled (WORKER_ENABLED=false)")
	}
	background = append(background, func() {
		go housekeepingLoop(ctx, st, log)
	})
	if cfg.BotConfigured() {
		bot := tgbot.New(svc, cfg, log)
		background = append(background, func() {
			go func() {
				if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("telegram bot stopped", "error", err)
				}
			}()
		})
	} else {
		log.Warn("telegram bot disabled: set TELEGRAM_BOT_ENABLED=true and TELEGRAM_BOT_TOKEN in .env")
	}

	for _, start := range background {
		start()
	}

	serverErr := make(chan error, 1)
	background = nil
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env,
			"cors", strings.Join(cfg.CORSOrigins, ","))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown", "error", err)
	}
	log.Info("stopped")
	return nil
}

// setOperatorPassword rotates the password of an existing operator, or creates
// the operator when the handle is new. The password never reaches the logs.
func setOperatorPassword(ctx context.Context, st *store.Store, operator, password, displayName string, log *slog.Logger) error {
	handle := strings.TrimSpace(operator)
	if !strings.HasPrefix(handle, "@") {
		handle = "@" + handle
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	if existing, err := st.GetOperator(ctx, handle); err == nil {
		if err := st.SetOperatorPassword(ctx, existing.ID, hash); err != nil {
			return err
		}
		log.Info("operator password updated", "operator", existing.ID)
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	name := displayName
	if name == "" {
		name = handle
	}
	if err := st.UpsertOperator(ctx, store.Operator{
		ID: handle, PasswordHash: hash, DisplayName: name, Active: true,
	}); err != nil {
		return err
	}
	log.Info("operator created", "operator", handle)
	return nil
}

// seedIfNeeded imports the demo data into an empty database.
// seedIfNeeded fills an empty database:
//
//   - SEED_ON_START=true (or -seed): the full demo dataset - orders, reviews,
//     promos and the demo operator. For demos and development.
//   - SEED_ON_START=false: only the base configuration (price list + settings
//     row), so a production install starts with working prices and no demo
//     content. A non-empty database is never touched.
func seedIfNeeded(ctx context.Context, st *store.Store, cfg *config.Config, svc *service.Service, log *slog.Logger, force bool) error {
	empty, err := st.IsEmpty(ctx)
	if err != nil {
		return err
	}
	if !empty {
		return nil
	}
	if cfg.SeedOnStart || force {
		return seed.Import(ctx, st, cfg.TokenPepper, log)
	}
	return seed.ImportBase(ctx, st, log)
}

// housekeepingLoop purges expired sessions and stale login attempts.
func housekeepingLoop(ctx context.Context, st *store.Store, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := st.PurgeExpiredSessions(ctx); err != nil {
				log.Error("housekeeping", "error", err)
			}
		}
	}
}
