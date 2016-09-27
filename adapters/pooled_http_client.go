package adapters

import (
	"io"
	"net/http"
	"sync/atomic"

	"github.com/abelyansky/pool"
)

// HttpClient represents behavior of a client within net/http package
type HttpClient interface {
	Do(req *http.Request) (resp *http.Response, err error)
	Get(url string) (*http.Response, error)
	Post(url string, bodyType string, body io.Reader) (*http.Response, error)
}

// PooledHttpClient is an adaper for standard net/http client which delegates to a pool under the hood
type PooledHttpClient struct {
	http.Client
	connPool         pool.Pool
	OutstandingConns int32
}

func NewPooledHttpCient(poolSize int, factory func() (* http.Client, error)) *PooledHttpClient {
	return &PooledHttpClient{connPool: pool}
}

func (c *PooledHttpClient) getConn() (conn *http.Client) {
	gcon, err := c.connPool.Get()
	if err != nil {
		panic(err)
	} else {
		atomic.AddInt32(&c.OutstandingConns, 1)
		return gcon.(*http.Client)
	}
}

func (c *PooledHttpClient) putConn(conn *http.Client) {
	c.connPool.Put(conn)
	atomic.AddInt32(&c.OutstandingConns, -1)
}

func (c *PooledHttpClient) Get(url string) (resp *http.Response, err error) {
	conn := c.getConn()
	defer c.putConn(conn)
	return conn.Get(url)
}

func (c *PooledHttpClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	conn := c.getConn()
	defer c.putConn(conn)
	resp, err = conn.Post(url, bodyType, body)
	return
}

func (c *PooledHttpClient) Do(req *http.Request) (resp *http.Response, err error) {
	conn := c.getConn()
	defer c.putConn(conn)
	resp, err = conn.Do(req)
	return
}

func (c *PooledHttpClient) Cleanup() {
	c.connPool.Close()
}
