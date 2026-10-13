// Package identity persists host identity transactionally before collection starts.
package identity

import (
	"errors"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
	"path/filepath"
	"time"
)

func Load(dir, configured string) (string, error) {
	db, e := bolt.Open(filepath.Join(dir, "identity.db"), 0600, &bolt.Options{Timeout: time.Second})
	if e != nil {
		return "", e
	}
	defer db.Close()
	var id string
	e = db.Update(func(tx *bolt.Tx) error {
		b, e := tx.CreateBucketIfNotExists([]byte("identity-v1"))
		if e != nil {
			return e
		}
		old := b.Get([]byte("id"))
		if old != nil {
			id = string(old)
			if id == "" {
				return errors.New("invalid host identity")
			}
			if configured != "" && configured != id {
				return errors.New("host identity change requires a new state directory")
			}
			return nil
		}
		id = configured
		if id == "" {
			id = uuid.NewString()
		}
		return b.Put([]byte("id"), []byte(id))
	})
	return id, e
}
