# Pool

Pool is a thread safe connection pool for any kinds of connections. It enforces blocking of the client requests when the pool is exhausted


## Install and Usage

Install the package with:

```bash
go get github.com/abelyansky/pool.v2
```

Import it with:

```go
import "github.com/abelyansky/pool.v2"
```

and use `pool` as the package name inside the code.

## Example

```go
// create a factory() to be used with channel based pool
factory    := func() (net.Conn, error) { return net.Dial("tcp", "127.0.0.1:4000") }

// create a new channel based pool with an initial capacity of 5 and maximum
// capacity of 30. The factory will create 30 initial connections and put them
// into the pool.
p, err := pool.NewChannelPool(30, factory)

// now you can get a connection from the pool, if there is no connection
// available it will block
conn, err := p.Get()

// do something with conn and put it back to the pool by closing the connection
// (this doesn't close the underlying connection instead it's putting it back
// to the pool).
conn.Close()

// close pool any time you want, this closes all the connections inside a pool
p.Close()

// currently available connections in the pool
current := p.Len()

// you can also use the default http.Client adapter which implements the methods
// of http.Client such as Do,Get,Post and delegates the call to the pool of http.Client instances
```


## Credits

 * [Fatih Arslan](https://github.com/fatih)
 * [sougou](https://github.com/sougou)
 * [abelyansky](https://github.com/abelyansky)

## License

The MIT License (MIT) - see LICENSE for more details
