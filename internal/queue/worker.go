package queue

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/engine"
	"git.myservermanager.com/varakh/yafe/internal/registry"
	"github.com/rs/zerolog/log"
)

type WorkerConfig struct {
	PollInterval    time.Duration
	CleanupInterval time.Duration
}

func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		PollInterval:    time.Second,
		CleanupInterval: time.Hour,
	}
}

type Worker struct {
	queue    Queue
	registry registry.FlowRegistry
	engine   engine.Executor
	config   WorkerConfig
	done     chan struct{}
	wg       sync.WaitGroup
}

func NewWorker(q Queue, reg registry.FlowRegistry, e engine.Executor, config WorkerConfig) *Worker {
	return &Worker{
		queue:    q,
		registry: reg,
		engine:   e,
		config:   config,
		done:     make(chan struct{}),
	}
}

// Run starts the worker loop. It blocks until ctx is canceled and the current job finishes.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)

	log.Info().Msgf("Worker started, polling every %s", w.config.PollInterval)

	if w.queue.CleanupEnabled() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.runCleanup(ctx)
		}()
	}

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Worker shutting down")
			w.wg.Wait()
			return
		case <-ticker.C:
			w.drainQueue(ctx)
		}
	}
}

// Done returns a channel that is closed when the worker has finished.
func (w *Worker) Done() <-chan struct{} {
	return w.done
}

func (w *Worker) drainQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		processed, err := w.processNext(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Error processing job")
			return
		}
		if !processed {
			return // Queue empty
		}
	}
}

func (w *Worker) runCleanup(ctx context.Context) {
	log.Info().Msgf("Cleanup routine started, running every %s", w.config.CleanupInterval)

	w.doCleanup()

	ticker := time.NewTicker(w.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.doCleanup()
		}
	}
}

func (w *Worker) doCleanup() {
	if cleaned, err := w.queue.RunCleanup(); err != nil {
		log.Error().Err(err).Msg("Cleanup failed")
	} else if cleaned > 0 {
		log.Info().Int("count", cleaned).Msg("Cleaned up old jobs")
	}
}

func (w *Worker) processNext(ctx context.Context) (bool, error) {
	job, err := w.queue.Dequeue()
	if err != nil {
		return false, fmt.Errorf("dequeue: %w", err)
	}
	if job == nil {
		return false, nil
	}

	log.Info().
		Str("job_id", job.ID).
		Str("flow", job.Flow).
		Msg("Processing job")

	flowPath := w.registry.Path(job.Flow)
	flow, err := w.engine.LoadFromFile(flowPath)
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to load flow")
		return true, w.queue.MarkFailed(job, 1, err.Error())
	}

	// Create log file for job output
	logPath := w.queue.LogFile(job.ID)
	//gosec:disable G304 -- Path is constructed from trusted queue directory
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to create log file")
		return true, w.queue.MarkFailed(job, 1, fmt.Sprintf("creating log file: %v", err))
	}
	defer logFile.Close() //nolint:errcheck

	if err := w.engine.Run(ctx, flow, engine.WithLogWriter(logFile), engine.WithInputs(job.Inputs)); err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Flow execution failed")
		return true, w.queue.MarkFailed(job, 1, err.Error())
	}

	log.Info().Str("job_id", job.ID).Msg("Job completed successfully")
	return true, w.queue.MarkDone(job, 0)
}
