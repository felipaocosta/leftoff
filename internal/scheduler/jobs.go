package scheduler

import (
	"errors"
	"strconv"
	"time"

	"github.com/felipaocosta/leftoff/internal/library"
)

// Job statuses.
const (
	JobRunning = "running"
	JobDone    = "done"
)

// maxJobs bounds the finished jobs kept in memory.
const maxJobs = 20

// ErrNotRunning means the scheduler was not started (Run), so there is no
// owner for background jobs.
var ErrNotRunning = errors.New("scheduler is not running")

// ErrJobNotFound means no job has that id (it may have been pruned).
var ErrJobNotFound = errors.New("job not found")

// Job is a background "refresh all" (RF-21) and its progress.
type Job struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Done        int        `json:"done"`
	Failed      int        `json:"failed"`
	NewChapters int        `json:"new_chapters"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

// StartRefreshAll refreshes every series in the background, ignoring the
// periodic schedule, and returns the job to poll. A job already running is
// returned instead of starting another one.
func (s *Scheduler) StartRefreshAll() (Job, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if s.base == nil || s.base.Err() != nil {
		return Job{}, ErrNotRunning
	}
	for _, id := range s.order {
		if j := s.jobs[id]; j.Status == JobRunning {
			return *j, nil
		}
	}
	series := s.lib.List()
	s.seq++
	job := &Job{ID: strconv.Itoa(s.seq), Status: JobRunning, Total: len(series), StartedAt: s.opts.Clock.Now()}
	s.jobs[job.ID] = job
	s.order = append(s.order, job.ID)
	s.prune()

	base := s.base
	go func() {
		s.refresh(base, series, func(r library.RefreshReport, err error) {
			s.jobsMu.Lock()
			defer s.jobsMu.Unlock()
			job.Done++
			job.NewChapters += r.NewChapters
			if err != nil {
				job.Failed++
			}
		})
		s.jobsMu.Lock()
		defer s.jobsMu.Unlock()
		now := s.opts.Clock.Now()
		job.Status, job.FinishedAt = JobDone, &now
	}()
	return *job, nil
}

// JobByID returns a snapshot of a job.
func (s *Scheduler) JobByID(id string) (Job, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	return *j, nil
}

// prune drops the oldest finished jobs beyond maxJobs. Callers hold jobsMu.
func (s *Scheduler) prune() {
	for len(s.order) > maxJobs {
		oldest := s.order[0]
		if s.jobs[oldest].Status == JobRunning {
			return
		}
		delete(s.jobs, oldest)
		s.order = s.order[1:]
	}
}
