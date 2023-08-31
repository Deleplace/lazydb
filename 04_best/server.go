package doublecheck

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

type Server struct {
	dbmap     map[string]*DB
	dbPending map[string]<-chan bool

	// dbLock guards both dbmap and dbPromises
	// All usages are expected to be very short, which
	// will enable high throughput and concurrent creations
	// of db connections.
	dbLock sync.RWMutex

	// count is the total number of connections created
	count atomic.Uint64
}

func NewServer() *Server {
	return &Server{
		dbmap:     make(map[string]*DB),
		dbPending: make(map[string]<-chan bool),
	}
}

func (s *Server) getDB(tenantID string) *DB {
	s.dbLock.RLock()
	db := s.dbmap[tenantID]
	pending := s.dbPending[tenantID]
	s.dbLock.RUnlock()

	if db == nil {
		if pending != nil {
			// This is a blocking operation
			<-pending
			// Now the connection has been created! (by another goroutine)
			s.dbLock.RLock()
			db = s.dbmap[tenantID]
			s.dbLock.RUnlock()
			if db == nil {
				panic("nil db :(")
			}
			return db
		}

		// Creation for this tenant has not started yet!
		// Let's do it
		s.dbLock.Lock()
		pending := make(chan bool)
		s.dbPending[tenantID] = pending
		s.count.Add(1)
		s.dbLock.Unlock() // <- unlock early

		db = Connect(tenantID) // This takes time...

		s.dbLock.Lock()
		s.dbmap[tenantID] = db
		s.dbPending[tenantID] = nil
		close(pending) // This unblocks any goroutine reading on it
		s.dbLock.Unlock()
	}
	return db
}

func (s *Server) GetMyData(w http.ResponseWriter, r *http.Request) {
	tenantID := r.FormValue("tenant")
	db := s.getDB(tenantID)
	// log.Printf("Serving request for tenant %q\n", tenantID)
	v := db.get("mykey")
	fmt.Fprintf(w, "value: %q", v)
}

func (s *Server) Count() uint64 {
	return s.count.Load()
}
