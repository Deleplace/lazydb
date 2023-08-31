package lock

import (
	"math/rand"
	"time"
)

//
// This code simulates a DB connection that takes
// between 50ms and 250ms to create.
//

func Connect(dbID string) *DB {
	d := time.Duration(50+rand.Intn(200)) * time.Millisecond
	time.Sleep(d)
	return &DB{
		ID: dbID,
	}
}

type DB struct {
	ID string
	// other fields...
}

func (db *DB) get(key string) string {
	return "value_for_" + key
}
