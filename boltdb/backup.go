package boltdb

import (
	"io"

	"github.com/boltdb/bolt"
)

func Backup(db *bolt.DB) *io.PipeReader {
	r, w := io.Pipe()
	go func() {
		err := db.View(func(tx *bolt.Tx) error {
			_, err := tx.WriteTo(w)
			return err
		})
		w.CloseWithError(err)
	}()

	return r
}
