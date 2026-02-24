package db

import (
	"sync"
	"time"
)

// LazyPool manages a single database connection with lazy-close behavior.
// The connection is kept open for a configurable idle timeout to allow
// reuse during periods of high activity, then automatically closed.
// It uses reference counting so the connection stays open while any
// caller is still using it.
type LazyPool struct {
	path    string
	schema  string // schema SQL to run on open
	conn    *DB
	refs    int
	timer   *time.Timer
	timeout time.Duration
	mu      sync.Mutex
}

// NewLazyPool creates a new LazyPool for the given database path.
// The schema SQL is executed when opening a new connection.
// The connection will be closed after the specified idle timeout.
func NewLazyPool(path, schema string, idleTimeout time.Duration) *LazyPool {
	return &LazyPool{
		path:    path,
		schema:  schema,
		timeout: idleTimeout,
	}
}

// Acquire returns the existing database connection or opens a new one.
// It cancels any pending close timer and increments the reference count.
// Callers must call Release() when done with the connection.
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
		p.refs++
		return p.conn, nil
	}

	// Open new connection
	conn, err := openWithSchema(p.path, p.schema)
	if err != nil {
		return nil, err
	}
	p.conn = conn
	p.refs = 1
	return conn, nil
}

// Release decrements the reference count and, when no more holders remain,
// schedules the connection to close after the idle timeout.
func (p *LazyPool) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.refs > 0 {
		p.refs--
	}

	if p.refs > 0 || p.conn == nil {
		return
	}

	// No more holders — start idle timer
	if p.timer != nil {
		p.timer.Stop()
	}

	p.timer = time.AfterFunc(p.timeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		// Only close if still idle (no new Acquire since timer started)
		if p.refs == 0 && p.conn != nil {
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

	p.refs = 0

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}

	return nil
}
