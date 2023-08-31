package naive

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Server struct {
	dbmap map[string]*DB

	// count is the total number of connections created
	count atomic.Uint64
}

func NewServer() *Server {
	return &Server{
		dbmap: make(map[string]*DB),
	}
}

func (s *Server) getDB(tenantID string) *DB {
	// Here we're reading/writing the dbmap without any synchronization.
	// This means getDB is not thread-safe, and concurrent calls would
	// be very racy.

	db := s.dbmap[tenantID]
	if db == nil {
		s.count.Add(1)
		db = Connect(tenantID)
		s.dbmap[tenantID] = db
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
