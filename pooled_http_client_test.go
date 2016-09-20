package pool

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

const testUrl = "http://localhost:7777/echo"

func do(cl *PooledHttpClient, sleepDur int, body string, respChan chan http.Response) {
	url := testUrl
	if sleepDur > 0 {
		sleepdurStr := strconv.Itoa(sleepDur)
		url += "?sleep=" + sleepdurStr
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(body)))
	if err != nil {
		panic(err)
	}
	resp, _ := cl.Do(req)
	fmt.Println("got response", resp)
	respChan <- *resp
}

func doPost(cl *PooledHttpClient, sleepDur int, body string, respChan chan http.Response) {
	url := testUrl
	if sleepDur > 0 {
		sleepdurStr := strconv.Itoa(sleepDur)
		url += "?sleep=" + sleepdurStr
	}
	fmt.Println("calling ", url)
	resp, err := cl.Post(url, "text/plain", bytes.NewReader([]byte(body)))

	if err != nil {
		fmt.Println("got error", err)
	} else {
		fmt.Println("got response", resp)
		respChan <- *resp
	}

}

func TestPooledHttpClient_Post(t *testing.T) {
	p, _ := NewChannelPool(3, factory)

	pooledClient := PooledHttpClient{connPool: p}

	respChannel := make(chan http.Response, 3)

	respReceived, timedOut := doExhaustPool(&pooledClient, doPost, 3, respChannel)
	assert.True(t, timedOut, "expected to timeout on third request")
	assert.Equal(t, 0, len(respChannel), "expected no responses but got %d", len(respChannel))
	time.Sleep(2 * time.Second)
	assert.Equal(t, 2, len(respChannel), "after waiting expected 2 responses but got %d", respReceived)
	assert.Equal(t, 2, pooledClient.connPool.Len(), "expected 2 connections to be returned to the pool, but see ", pooledClient.connPool.Len())
}

func TestPooledHttpClient_Do(t *testing.T) {
	p, _ := NewChannelPool(3, factory)

	pooledClient := PooledHttpClient{connPool: p}

	respChannel := make(chan http.Response, 3)

	respReceived, timedOut := doExhaustPool(&pooledClient, do, 3, respChannel)
	assert.True(t, timedOut, "expected to timeout on third request")
	assert.Equal(t, 0, len(respChannel), "expected no responses but got %d", len(respChannel))
	time.Sleep(2 * time.Second)
	assert.Equal(t, 2, len(respChannel), "after waiting expected 2 responses but got %d", respReceived)
	assert.Equal(t, 2, pooledClient.connPool.Len(), "expected 2 connections to be returned to the pool, but see ", pooledClient.connPool.Len())
}

func TestPooledHttpClient_Swarm(t *testing.T) {
	StartHTTPServer()

	p, _ := NewChannelPool(2, factory)

	pooledClient := PooledHttpClient{connPool: p}

	var wg sync.WaitGroup
	for cnt := 10; cnt > 0; cnt-- {
		go func() {
			wg.Add(1)
			respChannel := make(chan http.Response, 1)
			doPost(&pooledClient, 1, "hello", respChannel)
			wg.Done()
		}()
	}

	time.Sleep(100 * time.Millisecond)
	fmt.Println("checking the assert for outstanding conns")
	assert.Equal(t, int32(2), pooledClient.OutstandingConns)
	wg.Wait()

}

func doExhaustPool(pooledClient *PooledHttpClient,
	doFn func(*PooledHttpClient, int, string, chan http.Response),
	poolCap int, respChannel chan http.Response) (int, bool) {
	msg := "hello"
	go doFn(pooledClient, 1, msg, respChannel)
	go doFn(pooledClient, 1, msg, respChannel)
	go doFn(pooledClient, 3, msg, respChannel)

	timeoutCtx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
	stopReading := false
	respReceived := 0
	timedOut := false

	for !stopReading {
		select {
		case <-respChannel:
			respReceived += 1
			if respReceived == 3 {
				stopReading = true
			}
		case <-timeoutCtx.Done():
			stopReading = true
			timedOut = true
		}
	}
	return respReceived, timedOut
}
