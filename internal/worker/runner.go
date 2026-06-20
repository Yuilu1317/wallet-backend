package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/types"
)

type Worker interface {
	RunOnce(ctx context.Context) error
}

type RunnerState string

const (
	RunnerStateStopped  RunnerState = "stopped"
	RunnerStateRunning  RunnerState = "running"
	RunnerStateStopping RunnerState = "stopping"
)

type Runner struct {
	name       string
	worker     Worker
	interval   time.Duration
	runTimeout time.Duration

	mu    sync.Mutex
	runMu sync.Mutex

	state  RunnerState
	cancel context.CancelFunc
	done   chan struct{}

	lastStartedAt  time.Time
	lastFinishedAt time.Time
	lastError      string
}

func NewRunner(name string, worker Worker, interval time.Duration, runTimeout time.Duration) (*Runner, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	if worker == nil {
		return nil, fmt.Errorf("worker is required")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("worker interval must be positive")
	}
	if runTimeout <= 0 {
		return nil, fmt.Errorf("worker run timeout must be positive")
	}
	return &Runner{
		name:       name,
		worker:     worker,
		interval:   interval,
		runTimeout: runTimeout,
		state:      RunnerStateStopped,
	}, nil
}

func (r *Runner) Start(rootCtx context.Context) error {
	if rootCtx == nil {
		return fmt.Errorf("root context is nil")
	}
	if err := rootCtx.Err(); err != nil {
		return fmt.Errorf("root context is already done: %w", err)
	}

	r.mu.Lock()

	if r.state == RunnerStateRunning {
		r.mu.Unlock()
		return fmt.Errorf("runner already running: %s", r.name)
	}
	if r.state == RunnerStateStopping {
		r.mu.Unlock()
		return fmt.Errorf("runner is stopping: %s", r.name)
	}

	runnerCtx, cancel := context.WithCancel(rootCtx)
	done := make(chan struct{})

	r.state = RunnerStateRunning
	r.cancel = cancel
	r.done = done
	r.lastError = ""

	r.mu.Unlock()

	go r.loop(runnerCtx, done)

	return nil
}

func (r *Runner) loop(runnerCtx context.Context, done chan struct{}) {
	log.Printf("[worker-runner] started: name=%s", r.name)

	defer func() {
		r.markStopped()
		close(done)
		log.Printf("[worker-runner] stopped: name=%s", r.name)
	}()

	r.runOnceAndLog(runnerCtx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-runnerCtx.Done():
			return

		case <-ticker.C:
			r.runOnceAndLog(runnerCtx)
		}
	}
}

func (r *Runner) runOnceAndLog(runnerCtx context.Context) {
	err := r.RunOnce(runnerCtx)
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, types.ErrRequestCanceled):
		log.Printf("[worker-runner] warn: canceled: name=%s err=%v", r.name, err)
	case errors.Is(err, types.ErrDBTimeout), errors.Is(err, types.ErrRunTimeout):
		log.Printf("[worker-runner] error: timeout: name=%s err=%v", r.name, err)
	default:
		log.Printf("[worker-runner] error: name=%s err=%v", r.name, err)
	}
}

func (r *Runner) RunOnce(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}

	r.runMu.Lock()
	defer r.runMu.Unlock()

	runCtx, cancel := context.WithTimeoutCause(ctx, r.runTimeout, types.ErrRunTimeout)
	defer cancel()

	r.mu.Lock()
	r.lastStartedAt = time.Now()
	r.lastError = ""
	r.mu.Unlock()

	err := r.worker.RunOnce(runCtx)

	r.mu.Lock()
	r.lastFinishedAt = time.Now()
	if err != nil {
		r.lastError = err.Error()
	}
	r.mu.Unlock()

	if err != nil {
		return fmt.Errorf("run worker once: name=%s: %w", r.name, err)
	}
	return nil
}

func (r *Runner) markStopped() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = RunnerStateStopped
	r.cancel = nil
	r.done = nil
}

func (r *Runner) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == RunnerStateStopped {
		return nil
	}

	if r.state == RunnerStateStopping {
		return nil
	}

	r.state = RunnerStateStopping

	if r.cancel != nil {
		r.cancel()
	}

	return nil
}

func (r *Runner) Wait(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}

	r.mu.Lock()
	done := r.done
	name := r.name
	r.mu.Unlock()

	if done == nil {
		return nil
	}

	select {
	case <-done:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("wait runner stopped: name=%s: %w", name, ctx.Err())
	}
}
