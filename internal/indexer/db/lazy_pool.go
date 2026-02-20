package db

import (
	"sync"
	"time"
)

// LazyPool manages a single database connection with lazy-close behavior.
// The connection is kept open for a configurable idle timeout to allow
// reuse during periods of high activity, then automatically closed.
type LazyPool struct {
	path    string
	conn    *DB
	timer   *time.Timer
	timeout time.Duration
	mu      sync.Mutex
}

// NewLazyPool creates a new LazyPool for the given database path.
// The connection will be closed after the specified idle timeout.
func NewLazyPool(path string, idleTimeout time.Duration) *LazyPool {
	return &LazyPool{
		path:    path,
		timeout: idleTimeout,
	}
}

// Acquire returns the existing database connection or opens a new one.
// It cancels any pending close timer. Callers must call Release() when
// done with the connection to allow lazy cleanup.
func (p *LazyPool) Acquire() (*DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Cancel any pending close
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}

	// Reuse existing connection
	if p.conn != nil {
		return p.conn, nil
	}

	// Open new connection
	conn, err := Open(p.path)
	if err != nil {
		return nil, err
	}
	p.conn = conn
	return conn, nil
}

// Release schedules the database connection to close after the idle timeout.
// This should be called after Acquire() when the caller is done with the connection.
func (p *LazyPool) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		return
	}

	// Cancel any existing timer
	if p.timer != nil {
		p.timer.Stop()
	}

	p.timer = time.AfterFunc(p.timeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.conn != nil {
			p.conn.Close()
			p.conn = nil
		}
		p.timer = nil
	})
}

// Close immediately closes any open connection and cancels pending timers.
// This should be called during shutdown.
func (p *LazyPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}

	return nil
}
