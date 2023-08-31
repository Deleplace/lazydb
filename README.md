# Lazy DB connection strategy for a multitenant app

Sample Go code of a server connecting to a database on-demand, depending on the incoming request.

The first time a request needs to access the database of a tenant T, the server initiation the new DB connection. After that,
subsequent requests for the same tenant T should reuse the connection, rather than creating a new one.

Here are benchmarks for 300 incoming requests, 4 distinct tenants, and maximum 60 requests hitting the server at the same time.

The database is fake: it just simulates a connection that takes between 50ms and 250ms to create.

## 1. Naive implementation

Uses a map of connections.

```
	db := s.dbmap[tenantID]
	if db == nil {
		db = Connect(tenantID)
		s.dbmap[tenantID] = db
	}
```

The benchmark handles the 300 requests in 283ms.

```
% go test -bench=.
BenchmarkServer-10    	       4	 283416740 ns/op
```

However, the code is not properly synchronized, causing random data race crashes!

```
fatal error: concurrent map writes
```

The race conditions are often detected in normal mode, and always detected when enabling the race detector using the `-race` flag.

## 2. Lock

Uses a [RWMutex](https://pkg.go.dev/sync#RWMutex) to guard the map:
- any number of requests are allowed to read the map concurrently
- only one request is allowed to write to the map at given time, when no request is reading it.

```
	s.dbmapLock.RLock()
	db := s.dbmap[tenantID]
	s.dbmapLock.RUnlock()

	if db == nil {
		s.dbmapLock.Lock()
		db = Connect(tenantID)
		s.dbmap[tenantID] = db
		s.dbmapLock.Unlock()
	}
```

The benchmark handles the 300 requests in 8082ms (8 seconds). 

It successfully passes the race detector.

```
% go test -bench=.
BenchmarkServer-10    	       1	8082930459 ns/op
```

Note that the lock is being held during the creation of the DB connection. This effectively serializes the creation of the connections, which are never created concurrently.

The code has a [TOCTOU](https://en.wikipedia.org/wiki/Time-of-check_to_time-of-use) problem: between the instant when you read the map and the instant when you create the connection, the connection may have been already created by another request. Many connections are created redundantly for the same tenant.

## 3. Double check

Performs a [double-checked locking](https://en.wikipedia.org/wiki/Double-checked_locking) before writing to the map, effectively preventing the unwanted creation of redundant connections.

```
	s.dbmapLock.RLock()
	db := s.dbmap[tenantID]
	s.dbmapLock.RUnlock()

	if db == nil {
		s.dbmapLock.Lock()
		db = s.dbmap[tenantID]
		if db == nil {
			db = Connect(tenantID)
			s.dbmap[tenantID] = db
		}
		s.dbmapLock.Unlock()
	}
```

The benchmark handles the 300 requests in 547ms.

```
% go test -v -bench=.
BenchmarkServer-10    	       2	 547159146 ns/op
```

Note that the connections for distinct tenants are still created sequentially, not concurrently.

## 4. Channels

In implementations 2 and 3, no request is allowed to make any progress while one request is holding the write lock for the whole duration of its connection creation. This is suboptimal, and problematic when you have a lot of tenants and a lot of incoming traffic.

We achieve more fluid concurrent requests with the following strategy:
- no lock is held by a request, during the creation of a connection,
- the lock must always be held for a very short time,
- a connection for a given tenant is either found, or absent, or "pending"
- when a request need a connection that is "pending", it must wait until the connection is created, without initiating a new creation.

The synchronization device for waiting on a "pending" connection may be a Mutex, or a Semaphore, or a channel.

```
	dbmap     map[string]*DB
	dbPending map[string]<-chan bool
	dbLock    sync.RWMutex
```

Our implementation creates a channel for each "pending" connection, and closes the channel when the connection becomes available, effectively broadcasting the message "your connection is ready" to several blocked requests.

See the code for `(*Server).getDB` in `04_best/server.go`.

The benchmark handles the 300 requests in 212ms.

```
% go test -v -bench=.
BenchmarkServer-10    	       8	 212337531 ns/op
```

In this artificial experiment, everything is local. The only operation that takes a lot of time is the simulation (with [time.Sleep](https://pkg.go.dev/time#Sleep)) of the creation of a database connection. Thus, it makes sense that the total duration of the benchmark is dominated by the time it takes to create 4 connections concurrently. This is the duration of "the slowest out of 4", which is smaller than the sum of 4 sequential creations.