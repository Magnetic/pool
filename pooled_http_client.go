package pool

import (
	"net/http"
	"io"
)


type PooledHttpClient struct {
	http.Client
	connPool Pool
}

func connFetcher(p Pool) (func () (* http.Client)) {
	return func() (conn * http.Client) {
		res, err := p.Get()
		if err != nil {
			panic(err)
		} else {
			defer p.Put(res)
		}

		return res.(*http.Client)
	}
}

func (c * PooledHttpClient) Get(url string) (resp * http.Response, err error) {
	conn := connFetcher(c.connPool)()
	return conn.Get(url)
}

func (c * PooledHttpClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	conn := connFetcher(c.connPool)()
	return conn.Post(url, bodyType, body)
}

func (c * PooledHttpClient) Do(req * http.Request) (* http.Response, error) {
	conn := connFetcher(c.connPool)()
	return conn.Do(req)
}