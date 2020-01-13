package commands

import (
	"context"
	"encoding/base64"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"
	"s3fc/s3"

	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/boltdb/bolt"
)

// WriteDestinationObject queries for sources objects by their destination
// object id and concatinates them as per partition and destination object
// configuration.
type WriteDestinationObject struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
	ID     string `json:"id"`

	client s3iface.S3API
	db     *bolt.DB
}

// Invoke triggers the WriteDestinationObject command
func (w WriteDestinationObject) Invoke(ctx context.Context) error {
	return w.db.View(func(tx *bolt.Tx) error {
		set := models.NewObjectSet(w.Bucket, w.Prefix)
		b, err := boltdb.LookupTable(tx, set)
		if err != nil {
			return err
		}

		id, err := base64.RawURLEncoding.DecodeString(w.ID)
		if err != nil {
			return err
		}

		dest := models.NewDestinationObject(*set)
		if err = boltdb.LookupRow(b, id, dest); err != nil {
			return err
		}

		index := []byte("idx_destination")
		prefix := id
		limit := 2048

		sources := make([]models.SourceObject, 0, limit)
		var exclusiveStart []byte
		for running := true; running; {
			ids, err := boltdb.PrefixQuery(
				b, index, prefix, limit, exclusiveStart,
			)
			if err != nil {
				return err
			}

			for _, sID := range ids {
				source := models.NewSourceObject(*set)
				if err = boltdb.LookupRow(b, sID, source); err != nil {
					return err
				}

				sources = append(sources, *source)
			}

			if len(ids) < limit {
				running = false
				continue
			}

			exclusiveStart = ids[len(ids)-1]
		}

		_, err = s3.MergeObjects(ctx, w.client, *dest, sources)
		return err
	})
}

// Dependencies initializes a new command instance for invocation
func (w *WriteDestinationObject) Dependencies(
	c base.Container,
) (err error) {
	w.client, err = c.S3API()
	if err != nil {
		return err
	}
	w.db, err = c.DB()

	return err
}
