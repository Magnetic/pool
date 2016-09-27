package pool

import (
	"errors"
	"fmt"
)

type GenericConn interface{}

// channelPool implements the Pool interface based on buffered channels.
type channelPool struct {
	// storage for our generic connections
	conns chan GenericConn

	// generator of generic connections
	factory Factory
}

// Factory is a function to create new connections.
type Factory func() (GenericConn, error)

// NewChannelPool returns a new pool based on buffered channels with an initial
// capacity fixed capacity. Factory is used to populate the pool upon creation
//
func NewChannelPool(maxCap int, factory Factory) (Pool, error) {
	c := &channelPool{
		conns:   make(chan GenericConn, maxCap),
		factory: factory,
	}

	// create initial connections, if something goes wrong,
	// just close the pool error out.
	for i := 0; i < maxCap; i++ {
		conn, err := factory()
		if err != nil {
			return nil, fmt.Errorf("factory is not able to fill the pool: %s", err)
		}
		c.conns <- conn
	}

	return c, nil
}

// Get implements the Pool interfaces Get() method. If there is no new
// connection available in the pool, the client blocks
func (c *channelPool) Get() (GenericConn, error) {
	if c.conns == nil {
		return nil, ErrClosed
	}

	select {
	case conn := <-c.conns:
		if conn == nil {
			return nil, ErrClosed
		}

		return conn, nil
	}
}

// put puts the connection back to the pool. If the pool is full or closed,
// conn is simply closed. A nil conn will be rejected.
func (c *channelPool) Put(conn GenericConn) error {
	if conn == nil {
		return errors.New("connection is nil. rejecting")
	}

	if c.conns == nil {
		// pool is closed, close passed connection
		return nil
	}

	// put the resource back into the pool. If the pool is full, this will
	// block and the default case will be executed.
	select {
	case c.conns <- conn:
		return nil
	default:
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
