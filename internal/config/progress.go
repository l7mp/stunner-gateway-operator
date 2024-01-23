package config

import "sync"

// ProgressReporter is a general interface ProgressTrackers must satisfy.
type ProgressReporter interface {
	ProgressReport() int
}

// ProgressTracker is an embeddable object that can track and report the number of ongoing
// operations for some parallel process. This can be used for implementing graceful shutdown.
type ProgressTracker struct {
	progress int
	lock     sync.RWMutex
}

// NewProgressTracker creates an empty progress tracker.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{}
}

// Update can be used to increase or decrease (for negative delta) the progress counter.
func (t *ProgressTracker) ProgressUpdate(delta int) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.progress += delta
}

// ProgressReport returns the number of operations in progress.
func (t *ProgressTracker) ProgressReport() int {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.progress
}
