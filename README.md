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

However, the code is not properly synchronized, causing random data races!

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

