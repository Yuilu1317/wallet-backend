package worker

import "time"

type RunnerStatus struct {
	Name           string      `json:"name"`
	State          RunnerState `json:"state"`
	Running        bool        `json:"running"`
	Interval       string      `json:"interval"`
	RunTimeout     string      `json:"run_timeout"`
	LastStartedAt  time.Time   `json:"last_started_at"`
	LastFinishedAt time.Time   `json:"last_finished_at"`
	LastError      string      `json:"last_error"`
}

func (r *Runner) Status() RunnerStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	return RunnerStatus{
		Name:           r.name,
		State:          r.state,
		Running:        r.state == RunnerStateRunning,
		Interval:       r.interval.String(),
		RunTimeout:     r.runTimeout.String(),
		LastStartedAt:  r.lastStartedAt,
		LastFinishedAt: r.lastFinishedAt,
		LastError:      r.lastError,
	}
}
