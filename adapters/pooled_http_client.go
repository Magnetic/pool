package adapters

import (
	"io"
	"net/http"
	"sync/atomic"
	"io/ioutil"
	"time"
	"bytes"
	
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
	timeout          time.Duration
	OutstandingConns int32
}

// HttpResponseBody is an adapter for body present within http.Respose
// it holds all of the data from the original body and presents the same
// io.Reader interface to the outside world so that this body can be used
// in the same way as the original
type HttpResponseBody struct {
	io.ReadCloser
	body io.Reader
	err  error
}

// read the entire content out so that we can close the body
func newBodyWrapper(del io.ReadCloser) *HttpResponseBody {
	response := &HttpResponseBody{}
	data, err := ioutil.ReadAll(del)
	del.Close()
	response.body = bytes.NewReader(data)
	response.err = err
	return response
}

func (w HttpResponseBody) Read(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	} else {
		return w.body.Read(p)
	}

}

func (w HttpResponseBody) Close() (err error) {
	return nil
}

func NewPooledHttpClientWithTimeout(poolSize int, factory func() (HttpClient, error), timeout time.Duration) (*PooledHttpClient, error) {
	client, err := NewPooledHttpClient(poolSize, factory)
	if err != nil {
		return client, err
	} else {
		client.timeout = timeout
		return client, err
	}
}

func NewPooledHttpClient(poolSize int, factory func() (HttpClient, error)) (*PooledHttpClient, error) {
	factoryWrapper := func() (pool.GenericConn, error) {
		inst, err := factory()
		if err == nil && inst != nil {
			return inst.(pool.GenericConn), err
		} else {
			return nil, err
		}
	}
	pool, err := pool.NewChannelPool(poolSize, factoryWrapper)

	return &PooledHttpClient{connPool: pool}, err
}

func (c *PooledHttpClient) getConn() (connHolder *pool.ConnectionHolder, err error) {
	if c.timeout > 0 {
		connHolder, err = c.connPool.GetWithTimeout(c.timeout)
	} else {
		connHolder, err = c.connPool.Get()
	}

	if err != nil {
		return connHolder, err
	} else {
		atomic.AddInt32(&c.OutstandingConns, 1)
		return connHolder, nil
	}
}

func (c *PooledHttpClient) putConn(conn *pool.ConnectionHolder) {
	if conn == nil || !conn.InUse {
		return
	}
	c.connPool.Put(conn)
	atomic.AddInt32(&c.OutstandingConns, -1)
}

func (c *PooledHttpClient) Get(url string) (resp *http.Response, err error) {
	connHolder, err := c.getConn()
	defer c.putConn(connHolder)

	if err != nil {
		return nil, err
	}
	resp, err = connHolder.Conn.(*http.Client).Get(url)
	if err != nil {
		return
	}
	resp.Body = newBodyWrapper(resp.Body)
	return
}

func (c *PooledHttpClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	connHolder, err := c.getConn()
	defer c.putConn(connHolder)
	if err != nil {
		return nil, err
	}
	resp, err = connHolder.Conn.(*http.Client).Post(url, bodyType, body)
	resp.Body = newBodyWrapper(resp.Body)
	return
}

func (c *PooledHttpClient) Do(req *http.Request) (resp *http.Response, err error) {
	connHolder, err := c.getConn()
	defer c.putConn(connHolder)
	if err != nil {
		return nil, err
	}
	resp, err = connHolder.Conn.(*http.Client).Do(req)
	resp.Body = newBodyWrapper(resp.Body)
	return
}

func (c *PooledHttpClient) Cleanup() {
	c.connPool.Close()
}
