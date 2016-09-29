package adapters

import (
	"io"
	"net/http"
	"sync/atomic"

	"bytes"
	"github.com/abelyansky/pool"
	"io/ioutil"
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

type BodyWrapper struct {
	io.ReadCloser
	body io.Reader
	err  error
}

func NewBodyWrapper(del io.ReadCloser) *BodyWrapper {
	response := &BodyWrapper{}
	data, err := ioutil.ReadAll(del)
	del.Close()
	response.body = bytes.NewReader(data)
	response.err = err
	return response
}

func (w BodyWrapper) Read(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	} else {
		return w.body.Read(p)
	}

}

func (w BodyWrapper) Close() (err error) {
	return nil
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

func (c *PooledHttpClient) getConn() (conn *pool.ConnectionHolder, err error) {
	holder, err := c.connPool.Get()
	if err != nil {
		return holder, err
	} else {
		atomic.AddInt32(&c.OutstandingConns, 1)
		return holder, nil
	}
}

func (c *PooledHttpClient) putConn(conn *pool.ConnectionHolder) {
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
	resp.Body = NewBodyWrapper(resp.Body)
	return
}

func (c *PooledHttpClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	connHolder, err := c.getConn()
	defer c.putConn(connHolder)
	resp, err = connHolder.Conn.(*http.Client).Post(url, bodyType, body)
	resp.Body = NewBodyWrapper(resp.Body)
	return
}

func (c *PooledHttpClient) Do(req *http.Request) (resp *http.Response, err error) {
	connHolder, err := c.getConn()
	defer c.putConn(connHolder)
	resp, err = connHolder.Conn.(*http.Client).Do(req)
	resp.Body = NewBodyWrapper(resp.Body)
	return
}

func (c *PooledHttpClient) Cleanup() {
	c.connPool.Close()
}
