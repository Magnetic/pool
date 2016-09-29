package adapters

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/abelyansky/pool"
)

const testUrl = "http://localhost:7777/echo"

var (
	address = "http://127.0.0.1:7777"

	maxPoolSize             = 3
	normalCallSleepDuration = time.Millisecond * 10
	longerCallSleepDuration = time.Millisecond * 20
	maxCallSleepDuration    = time.Millisecond * 30

	factory = func() (pool.GenericConn, error) {
		return &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1,
				// to make a point that this is what we want
				DisableKeepAlives:     false,
				ExpectContinueTimeout: 15 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
			},
			Timeout: 15 * time.Second,
		}, nil
	}

	httpClientFactory = func() (client HttpClient, err error) {
		conn, err := factory()
		client = conn.(HttpClient)
		return client, err
	}

	once sync.Once
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	StartHTTPServer()
}

func StartHTTPServer() {
	once.Do(func() {
		// used for factory function
		go simpleHTTPServer()
		time.Sleep(time.Millisecond * 300) // wait until tcp server has been settled

	})

}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("ServeHTTP called")

	queryArgs := r.URL.Query()

	data, err := ioutil.ReadAll(r.Body)
	if err == nil {
		defer r.Body.Close()
	}

	if sleepDur, ok := queryArgs["sleep"]; ok {
		sleepDurNanos, err := strconv.Atoi(sleepDur[0])
		if err != nil {
			panic(err)
		}
		fmt.Println("sleeping for ", sleepDurNanos/1000000000, "seconds")
		time.Sleep(time.Duration(sleepDurNanos) * time.Nanosecond)
	}
	w.Write(data)
}

func simpleHTTPServer() {
	http.HandleFunc("/echo", ServeHTTP)

	srv := &http.Server{
		Addr:         ":7777",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	err := srv.ListenAndServe()

	if err != nil {
		log.Fatal(err)
	}
}

func do(cl *PooledHttpClient, sleepDur time.Duration, body string, respChan chan http.Response) {
	url := testUrl
	if sleepDur > 0 {
		sleepdurStr := strconv.Itoa(int(sleepDur.Nanoseconds()))
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

func doPost(cl *PooledHttpClient, sleepDur time.Duration, body string, respChan chan http.Response) {
	url := testUrl
	if sleepDur > 0 {
		sleepdurStr := strconv.Itoa(int(sleepDur.Nanoseconds()))
		url += "?sleep=" + sleepdurStr
	}
	fmt.Println("calling ", url)
	resp, err := cl.Post(url, "text/plain", bytes.NewReader([]byte(body)))

	if err != nil {
		fmt.Println("got error", err)
		panic(err)
	} else {
		fmt.Println("got response", resp)
		respChan <- *resp
	}

}

func TestConnPost(t *testing.T) {
	p, _ := pool.NewChannelPool(30, factory)

	connHolder, _ := p.Get()

	msg := "hello"
	resp, err := connHolder.Conn.(*http.Client).Post("http://localhost:7777/echo", "text/plain", bytes.NewReader([]byte(msg)))
	if err != nil {
		t.Error(err)
	}
	respMsg, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
	}
	defer resp.Body.Close()

	if msg != string(respMsg) {
		t.Errorf("Expected response %s but got %s ", msg, resp)
	}
}

func TestPooledHttpClient_Post(t *testing.T) {
	p, _ := pool.NewChannelPool(maxPoolSize, factory)

	pooledClient := PooledHttpClient{connPool: p}

	respChannel := make(chan http.Response, maxPoolSize)

	doExhaustPool(&pooledClient, doPost, maxPoolSize, respChannel)
	respReceived, timedOut := getFastResponses(maxPoolSize, respChannel)
	assert.True(t, timedOut, "expected to timeout on last slow request")
	assert.Equal(t, maxPoolSize-1, respReceived)
	assert.Equal(t, maxPoolSize-1, pooledClient.connPool.Len()) // slow connection is still busy
	time.Sleep(longerCallSleepDuration)                         // wait for slow request to return
	assert.Equal(t, 1, len(respChannel))
	assert.Equal(t, maxPoolSize, pooledClient.connPool.Len()) // all conns back in the pool
}

func TestPooledHttpClient_Do(t *testing.T) {
	p, _ := pool.NewChannelPool(maxPoolSize, factory)

	pooledClient := PooledHttpClient{connPool: p}

	respChannel := make(chan http.Response, maxPoolSize)

	doExhaustPool(&pooledClient, do, maxPoolSize, respChannel)
	respReceived, timedOut := getFastResponses(maxPoolSize, respChannel)
	assert.True(t, timedOut, "expected to timeout on last, slow request")
	assert.Equal(t, maxPoolSize-1, respReceived)
	assert.Equal(t, maxPoolSize-1, pooledClient.connPool.Len()) // slow connection is still busy
	time.Sleep(longerCallSleepDuration)                         // wait for slow request to return
	assert.Equal(t, 1, len(respChannel))
	assert.Equal(t, maxPoolSize, pooledClient.connPool.Len()) // all conns back in the pool
}

// TestPooledHttpClient_Swarm tests
func TestPooledHttpClient_Swarm(t *testing.T) {
	StartHTTPServer()

	pool, _ := pool.NewChannelPool(2, factory)
	pooledClient := PooledHttpClient{connPool: pool}

	var wg sync.WaitGroup
	responses := 0
	for cnt := 10; cnt > 0; cnt-- {
		go func() {
			wg.Add(1)
			respChannel := make(chan http.Response, 1)
			doPost(&pooledClient, longerCallSleepDuration, "hello", respChannel)
			wg.Done()
			responses += 1
		}()
	}

	time.Sleep(normalCallSleepDuration)
	// verify that two connections are in use
	assert.Equal(t, int32(2), pooledClient.OutstandingConns)
	// verify no responses yet
	assert.Equal(t, 0, responses)
	wg.Wait() // after this all responses should come in
	assert.Equal(t, 10, responses)
	assert.Equal(t, 0, int(pooledClient.OutstandingConns))

}

func TestLargeStringRead(t *testing.T) {
	respChannel := make(chan http.Response, 1)
	pooledClient, _ := NewPooledHttpClient(2, httpClientFactory)
	largeString := generateRandomString(1024 * 1000)
	assert.Equal(t, 1024*1000, len(largeString))
	doPost(pooledClient, longerCallSleepDuration, largeString, respChannel)
	resp := <-respChannel
	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)
	assert.Equal(t, largeString, string(body))
}

// getFastResponses waits for normal responses to come to the channel,
// but not for the slow outlier
func getFastResponses(poolCap int, respChannel chan http.Response) (int, bool) {

	timeoutCtx, _ := context.WithTimeout(context.Background(), longerCallSleepDuration)
	stopReading := false
	respReceived := 0
	timedOut := false

	for !stopReading {
		select {
		case <-respChannel:
			respReceived += 1
			if respReceived == poolCap {
				stopReading = true
			}
		case <-timeoutCtx.Done():
			stopReading = true
			timedOut = true
		}
	}
	return respReceived, timedOut
}

// doExhaustPool requests all conns from the pool and makes a request on each
// the last request is made to be extra slow to simulate an outlier
func doExhaustPool(pooledClient *PooledHttpClient,
	doFn func(*PooledHttpClient, time.Duration, string, chan http.Response),
	poolCap int, respChannel chan http.Response) {
	msg := "hello"

	for c := 0; c < poolCap; c++ {
		dur := normalCallSleepDuration
		if c == poolCap-1 {
			dur = maxCallSleepDuration
		}
		go doFn(pooledClient, dur, msg, respChannel)
	}
}

// borrowed from http://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func generateRandomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}
