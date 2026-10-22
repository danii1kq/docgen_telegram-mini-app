// Package worker runs the generation queue: queued -> generating -> completed.
// It respects settings.queue_paused, expires unpaid orders, recovers orders
// stuck in generating and can run several AI agents in parallel - one order per
// fresh working directory (see agent.go).
package worker

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

// Worker processes the order queue.
type Worker struct {
	svc       *service.Service
	log       *slog.Logger
	generator Generator

	// sem bounds how many orders are generated at the same time.
	sem chan struct{}
	wg  sync.WaitGroup

	ctx context.Context
}

// New builds the worker and resolves the generator (agent or built-in).
func New(svc *service.Service, log *slog.Logger) *Worker {
	maxAgents := svc.Cfg.Worker.MaxAgents
	if maxAgents < 1 {
		maxAgents = 1
	}
	return &Worker{
		svc:       svc,
		log:       log,
		generator: resolveGenerator(svc),
		sem:       make(chan struct{}, maxAgents),
	}
}

// Run loops until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.ctx = ctx
	interval := w.svc.Cfg.Worker.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.log.Info("generation worker started",
		"interval", interval.String(),
		"max_agents", cap(w.sem),
		"retries", w.svc.Cfg.Worker.AgentRetries)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("generation worker stopped")
			return
		case <-ticker.C:
			if err := w.Tick(ctx); err != nil {
				w.log.Error("worker tick failed", "error", err)
			}
		}
	}
}

// Wait blocks until the running agents finish (used on shutdown and in tests).
func (w *Worker) Wait() { w.wg.Wait() }

// Tick performs one pass: maintenance tasks, then starts as many generations as
// the agent slots allow.
func (w *Worker) Tick(ctx context.Context) error {
	settings, err := w.svc.Store.GetSettings(ctx)
	if err != nil {
		return err
	}

	// Housekeeping, always on.
	if removed, err := w.svc.Store.ExpireOldResultFiles(ctx); err != nil {
		return err
	} else {
		for _, file := range removed {
			w.removeResultFile(file.StorageKey)
		}
	}
	if expired, err := w.svc.Store.ExpireUnpaidOrders(ctx, w.svc.Cfg.Worker.ExpireAfter); err != nil {
		return err
	} else if len(expired) > 0 {
		w.log.Info("orders expired", "count", len(expired))
		for _, number := range expired {
			if order, err := w.svc.Store.GetOrder(ctx, number); err == nil {
				w.svc.NotifyExpired(ctx, order)
			}
		}
	}
	if requeued, err := w.svc.Store.RequeueStaleGenerating(ctx, staleGeneratingAfter(w.svc.Cfg.Worker)); err != nil {
		return err
	} else if len(requeued) > 0 {
		w.log.Warn("stale generating orders requeued", "count", len(requeued))
	}

	if settings.QueuePaused {
		return nil
	}

	// Fill the free agent slots.
	for w.tryAcquire() {
		order, jobID, err := w.svc.Store.TakeNextQueued(ctx)
		if err != nil {
			w.release()
			if errors.Is(err, store.ErrNotFound) {
				return nil // queue empty
			}
			return err
		}
		w.wg.Add(1)
		go func(order *domain.Order, jobID int64) {
			defer w.wg.Done()
			defer w.release()
			w.generate(order, jobID)
		}(order, jobID)
	}
	return nil
}

// tryAcquire reserves an agent slot without blocking.
func (w *Worker) tryAcquire() bool {
	select {
	case w.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// staleGeneratingAfter is how long an order may sit in `generating` before the
// worker assumes the run died and requeues it. It must outlast the worst case -
// every attempt running to the timeout - otherwise a slow order is requeued
// while its agent is still working, and two runs race for the same order (and
// for the same working directory). WORKER_STALE_GENERATING_AFTER raises the
// threshold, never lowers it below that floor.
func staleGeneratingAfter(cfg config.WorkerConfig) time.Duration {
	attempts := cfg.AgentRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	// One extra minute per attempt: the runner's own context adds a minute on
	// top of GENERATOR_TIMEOUT before it reports the timeout error.
	floor := time.Duration(attempts)*(cfg.GeneratorTimeout+time.Minute) + time.Minute
	if cfg.StaleGeneratingAfter > floor {
		return cfg.StaleGeneratingAfter
	}
	return floor
}

func (w *Worker) release() { <-w.sem }

// generate runs the generator for one order, retrying transient failures, and
// always finishes the order (completed or failed_final) with a job log.
func (w *Worker) generate(order *domain.Order, jobID int64) {
	cfg := w.svc.Cfg.Worker
	base := w.ctx
	if base == nil {
		base = context.Background()
	}

	attempts := cfg.AgentRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastLog string
	for attempt := 1; attempt <= attempts; attempt++ {
		// One extra minute on top of the agent budget: the timeout inside the
		// runner reports a proper error first.
		ctx, cancel := context.WithTimeout(base, cfg.GeneratorTimeout+time.Minute)
		file, logText, err := w.generator.Generate(ctx, order)
		cancel()

		if err == nil {
			w.finishSuccess(ctx, order, jobID, file, logText, attempt)
			return
		}
		lastErr, lastLog = err, logText
		w.log.Warn("generation attempt failed",
			"order", order.Number, "attempt", attempt, "of", attempts, "error", err)
		if base.Err() != nil {
			break // shutting down: leave the order for the stale requeue
		}
	}

	w.finishFailure(base, order, jobID, lastLog, lastErr)
}

func (w *Worker) finishSuccess(ctx context.Context, order *domain.Order, jobID int64, files []store.OrderFileRow, logText string, attempt int) {
	// Use a fresh context: the agent context may be about to expire.
	done, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Every document of the run carries the attempt, so the newest set is what
	// the client and the panel see (an older set survives a rework).
	for i := range files {
		files[i].Attempt = attempt
	}

	completed, err := w.svc.Store.CompleteGeneration(done, order.Number, files, logText)
	if err != nil {
		w.log.Error("worker: complete order", "order", order.Number, "error", err)
		_ = w.svc.Store.FinishJob(done, jobID, "failed", err.Error())
		return
	}
	jobLog := logText
	if attempt > 1 {
		jobLog = logText + " (попытка " + strconv.Itoa(attempt) + ")"
	}
	_ = w.svc.Store.FinishJob(done, jobID, "done", jobLog)
	var totalBytes int64
	for _, file := range files {
		totalBytes += file.SizeBytes
	}
	w.log.Info("generation finished",
		"order", completed.Number, "files", len(files), "bytes", totalBytes, "attempt", attempt)
	w.svc.NotifyCompleted(done, completed)
}

func (w *Worker) finishFailure(ctx context.Context, order *domain.Order, jobID int64, logText string, cause error) {
	done, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reason := logText
	if reason == "" && cause != nil {
		reason = cause.Error()
	}
	_ = w.svc.Store.FinishJob(done, jobID, "failed", reason)
	failed, err := w.svc.Store.FailGeneration(done, order.Number, reason)
	if err != nil {
		w.log.Error("worker: fail order", "order", order.Number, "error", err)
		return
	}
	w.log.Error("generation failed", "order", order.Number, "reason", reason)
	w.svc.NotifyFailed(done, failed, reason)
}

// removeResultFile deletes an expired document from disk (best effort).
func (w *Worker) removeResultFile(storageKey string) {
	if storageKey == "" {
		return
	}
	if err := removeFile(w.svc.StoragePath(storageKey)); err != nil {
		w.log.Warn("worker: remove expired result", "key", storageKey, "error", err)
	}
}
