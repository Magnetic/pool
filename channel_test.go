package pool

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"
)

var (
	MaximumCap = 30

	factory = func() (GenericConn, error) {
		return "", nil
	}
)

func TestNew(t *testing.T) {
	_, err := newChannelPool()
	if err != nil {
		t.Errorf("New error: %s", err)
	}
}
func TestPool_Get_Impl(t *testing.T) {
	p, _ := newChannelPool()
	defer p.Close()

	connHolder, err := p.Get()
	if err != nil {
		t.Errorf("Get error: %s", err)
	}

	_, ok := connHolder.Conn.(GenericConn)
	if !ok {
		t.Errorf("Conn is not of type ConnectionHolder")
	}
}

func TestPool_Get(t *testing.T) {
	p, _ := newChannelPool()
	defer p.Close()

	_, err := p.Get()
	if err != nil {
		t.Errorf("Get error: %s", err)
	}

	// after one get, current capacity should be lowered by one.
	if p.Len() != (MaximumCap - 1) {
		t.Errorf("Get error. Expecting %d, got %d",
			(MaximumCap - 1), p.Len())
	}

	// exhaust he pool
	var wg sync.WaitGroup
	for i := 0; i < (MaximumCap - 1); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Get()
			if err != nil {
				t.Errorf("Get error: %s", err)
			}
		}()
	}
	wg.Wait()

	if p.Len() != 0 {
		t.Errorf("Get error. Expecting %d, got %d",
			(MaximumCap - 1), p.Len())
	}

	// confirm that requesting a new connection from an empty pool results in a wait

	waitMillis := 30
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(waitMillis)*time.Millisecond)
	connChannel := make(chan GenericConn)
	errorChannel := make(chan error)
	timedOut := false

	go func() {
		conn, err := p.Get()
		if err != nil {
			errorChannel <- err
		} else {
			connChannel <- conn
		}
	}()
	select {
	case <-connChannel:
		timedOut = false

	case <-errorChannel:
		timedOut = false

	case <-ctx.Done():
		timedOut = true
	}
	if !timedOut {
		t.Errorf("Got a connection after pool was exhausted: %s", err)
	}
}

func TestPool_Put(t *testing.T) {
	p, err := NewChannelPool(30, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// get/create from the pool
	conns := make([]*ConnectionHolder, MaximumCap)
	for i := 0; i < MaximumCap; i++ {
		conn, _ := p.Get()
		conns[i] = conn
	}

	// now put them all back
	for _, conn := range conns {
		p.Put(conn)
	}

	if p.Len() != MaximumCap {
		t.Errorf("Put error len. Expecting %d, got %d",
			1, p.Len())
	}

	p.Close() // close pool

	conn, _ := factory()
	p.Put(NewConnectionHolder(conn))
	if p.Len() != 0 {
		t.Errorf("Put error. Closed pool shouldn't allow to put connections.")
	}
}

func TestPool_UsedCapacity(t *testing.T) {
	p, _ := newChannelPool()
	defer p.Close()

	if p.Len() != MaximumCap {
		t.Errorf("InitialCap error. Expecting %d, got %d",
			MaximumCap, p.Len())
	}
}

func TestPool_Close(t *testing.T) {
	p, _ := newChannelPool()

	// now close it and test all cases we are expecting.
	p.Close()

	c := p.(*channelPool)

	if c.conns != nil {
		t.Errorf("Close error, conns channel should be nil")
	}

	if c.factory != nil {
		t.Errorf("Close error, factory should be nil")
	}

	_, err := p.Get()
	if err == nil {
		t.Errorf("Close error, get conn should return an error")
	}

	if p.Len() != 0 {
		t.Errorf("Close error used capacity. Expecting 0, got %d", p.Len())
	}
}

func TestPoolConcurrent(t *testing.T) {
	p, _ := newChannelPool()
	pipe := make(chan *ConnectionHolder, 0)

	go func() {
		p.Close()
	}()

	for i := 0; i < MaximumCap; i++ {
		go func() {
			conn, _ := p.Get()

			pipe <- conn
		}()

		go func() {
			conn := <-pipe
			if conn == nil {
				return
			}
			p.Put(conn)
		}()
	}
}

func TestPoolConcurrent2(t *testing.T) {
	p, _ := NewChannelPool(30, factory)

	var wg sync.WaitGroup

	go func() {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				conn, _ := p.Get()
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
				p.Put(conn)
				wg.Done()
			}(i)
		}
	}()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			conn, _ := p.Get()
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
			p.Put(conn)
			wg.Done()
		}(i)
	}

	wg.Wait()
}

func newChannelPool() (Pool, error) {
	return NewChannelPool(MaximumCap, factory)
}
