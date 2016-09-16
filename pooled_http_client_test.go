package pool

import (
	"net/http"
	"testing"
	"bytes"
	"golang.org/x/net/context"
	"time"
	"github.com/stretchr/testify/assert"
	"fmt"
	"strconv"
)

const testUrl = "http://localhost:7777/echo"

func do(cl * PooledHttpClient, sleepDur int, body string, respChan chan http.Response) {
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

func doPost (cl * PooledHttpClient, sleepDur int, body string, respChan chan http.Response) {
	url := testUrl
	if sleepDur > 0 {
		sleepdurStr := strconv.Itoa(sleepDur)
		url += "?sleep=" + sleepdurStr
	}
	resp, _ := cl.Post(url, "text/plain", bytes.NewReader([]byte(body)))
	fmt.Println("got response", resp)
	respChan <- *resp
}


func TestPooledHttpClient_Post(t *testing.T) {
	p, _ := NewChannelPool(2, 3, factory)

	pooledClient := PooledHttpClient{connPool: p}

	respChannel := make (chan http.Response, 3)

	respReceived, timedOut := doExhaustPool(&pooledClient, doPost, 3, respChannel)
	assert.True(t, timedOut, "expected to timeout on third request")
	assert.Equal(t, 0, len(respChannel), "expected no responses but got %d", len(respChannel))
	time.Sleep(1 * time.Second)
	assert.Equal(t, 2, len(respChannel), "after waiting expected 2 responses but got %d", respReceived)
	assert.Equal(t, 2, pooledClient.connPool.Len(), "expected 2 connections to be returned to the pool, but see ", pooledClient.connPool.Len())
}

func TestPooledHttpClient_Do(t *testing.T) {
	p, _ := NewChannelPool(2, 3, factory)

	pooledClient := PooledHttpClient{connPool: p}

	respChannel := make (chan http.Response, 3)

	respReceived, timedOut := doExhaustPool(&pooledClient, do, 3, respChannel)
	assert.True(t, timedOut, "expected to timeout on third request")
	assert.Equal(t, 0, len(respChannel), "expected no responses but got %d", len(respChannel))
	time.Sleep(1 * time.Second)
	assert.Equal(t, 2, len(respChannel), "after waiting expected 2 responses but got %d", respReceived)
	assert.Equal(t, 2, pooledClient.connPool.Len(), "expected 2 connections to be returned to the pool, but see ", pooledClient.connPool.Len())
}


func doExhaustPool(pooledClient * PooledHttpClient,
                   doFn func (*PooledHttpClient, int, string, chan http.Response),
                    poolCap int, respChannel chan http.Response) (int, bool) {
	msg := "hello"
	go doFn(pooledClient, 1, msg, respChannel)
	go doFn(pooledClient, 1, msg, respChannel)
	go doFn(pooledClient, 3, msg, respChannel)


	timeoutCtx, _ := context.WithTimeout(context.Background(), 10 * time.Millisecond)
	stopReading := false
	respReceived := 0
	timedOut := false

	for ;!stopReading; {
		select {
		case <- respChannel:
			respReceived += 1
			if respReceived == 3 {
				stopReading = true
			}
		case <- timeoutCtx.Done():
			stopReading = true
			timedOut = true
		}
	}
	return respReceived, timedOut
}