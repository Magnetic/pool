package pool

import (
	"errors"
	"fmt"
	"time"
)

// channelPool implements the Pool interface based on buffered channels.
type channelPool struct {
	// storage for our generic connections
	conns chan *ConnectionHolder

	// generator of generic connections
	factory Factory
	maxCap  int
}

// Factory is a function to create new connections.
type Factory func() (GenericConn, error)

// NewChannelPool returns a new pool based on buffered channels with an initial
// capacity fixed capacity. Factory is used to populate the pool upon creation
//
func NewChannelPool(maxCap int, factory Factory) (Pool, error) {
	c := &channelPool{
		conns:   make(chan *ConnectionHolder, maxCap),
		factory: factory,
		maxCap:  maxCap,
	}

	// create initial connections, if something goes wrong,
	// just close the pool error out.
	for i := 0; i < maxCap; i++ {
		conn, err := factory()
		if err != nil {
			return nil, fmt.Errorf("factory is not able to fill the pool: %s", err)
		}
		c.conns <- NewConnectionHolder(conn)
	}

	return c, nil
}

// Get implements the Pool interfaces Get() method. If there is no new
// connection available in the pool, the client blocks
func (c *channelPool) Get() (*ConnectionHolder, error) {
	if c.conns == nil {
		return nil, ErrClosed
	}

	select {
	case conn := <-c.conns:
		if conn == nil {
			return nil, ErrClosed
		}
		conn.InUse = true

		return conn, nil
	}
}

func (c *channelPool) GetWithTimeout(timeout time.Duration) (*ConnectionHolder, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	if c.conns == nil {
		return nil, ErrClosed
	}

	select {
	case conn := <-c.conns:
		if conn == nil {
			return nil, ErrClosed
		}
		conn.InUse = true

		return conn, nil
	case <-timer.C:
		return nil, ErrTimedOut
	}
}

// put puts the connection back to the pool. If the pool is full or closed,
// conn is simply closed. A nil conn will be rejected.
func (c *channelPool) Put(conn *ConnectionHolder) error {
	if conn == nil {
		return errors.New("connection is nil. rejecting")
	}

	if c.conns == nil || !conn.InUse {
		// pool is closed, close passed connection
		return nil
	}

	// put the resource back into the pool. This code will block if
	// the capacity of the pool is full, but the checks above will prevent
	// that scenario
	select {
	case c.conns <- conn:
		conn.InUse = false
		return nil
	}
}

func (c *channelPool) Len() int { return len(c.conns) }

func (c *channelPool) Close() {
	if c.conns != nil && len(c.conns) > 0 {
		_, isPoolOpen := <-c.conns
		if !isPoolOpen {
			close(c.conns)
		}

	}
	c.conns = nil
	c.factory = nil
}
