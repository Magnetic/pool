// Package pool implements a pool of net.Conn interfaces to manage and reuse them.
package pool

import (
	"errors"
	"time"
)

var (
	// ErrClosed is the error resulting if the pool is closed via pool.Close().
	ErrClosed   = errors.New("pool is closed")
	ErrTimedOut = errors.New("timed out waiting for connection")
)

type GenericConn interface{}

type ConnectionHolder struct {
	Conn  GenericConn
	InUse bool
}

func NewConnectionHolder(conn GenericConn) *ConnectionHolder {
	return &ConnectionHolder{Conn: conn}
}

// Pool interface describes a pool implementation. A pool should have maximum
// capacity. An ideal pool is threadsafe and easy to use.
type Pool interface {
	// Get returns a new connection from the pool. Closing the connections puts
	// it back to the Pool. Closing it when the pool is destroyed or full will
	// be counted as an error.
	Get() (*ConnectionHolder, error)

	GetWithTimeout(time.Duration) (*ConnectionHolder, error)

	Put(*ConnectionHolder) error
	// Close closes the pool and all its connections. After Close() the pool is
	// no longer usable.
	Close()

	// Len returns the current number of connections of the pool.
	Len() int
}
