package commands

import (
	"context"
	"encoding/base64"
	"errors"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"
	"strings"

	"github.com/boltdb/bolt"
)

var (
	// ErrMissingDelimeter tells a caller that a request is missing required
	// parameters, delimiter or delimiter_b64
	ErrMissingDelimeter = errors.New("Missing required parameters, delimiter or delimiter_b64")
)

// PutObjectSet ensures an object set exists in a bolt database and will also
// update schema, indexes, and metadata
type PutObjectSet struct {
	Bucket            string  `json:"bucket"`
	Prefix            string  `json:"prefix"`
	DestinationBucket string  `json:"destination_bucket"`
	DestinationPath   string  `json:"destination_path"`
	BlockSize         int64   `json:"block_size"`
	Delimiter         *string `json:"delimiter,omitempty"`
	DelimiterB64      *string `json:"delimiter_b64,omitempty"`

	db *bolt.DB
}

// Invoke triggers the PutObjectSet command
func (p PutObjectSet) Invoke(ctx context.Context) error {
	var err error
	objectSet := models.NewObjectSet(p.Bucket, p.Prefix)

	objectSet.BlockSize = p.BlockSize
	objectSet.DestinationBucket = p.DestinationBucket
	objectSet.DestinationPath = strings.Trim(p.DestinationPath, "/") + "/"

	if p.Delimiter != nil {
		objectSet.Delimiter = []byte(*p.Delimiter)
	} else if p.DelimiterB64 != nil {
		objectSet.Delimiter, err = base64.StdEncoding.DecodeString(*p.DelimiterB64)
		if err != nil {
			return err
		}
	} else {
		return ErrMissingDelimeter
	}

	if err = boltdb.EnsureTable(p.db, objectSet); err != nil {
		return err
	}

	return p.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(objectSet.Name())
		schema := objectSet.Schema()
		values, err := objectSet.Marshal()
		if err != nil {
			return err
		}
		for k, v := range values {
			if err = b.Put(schema[k], v); err != nil {
				return err
			}
		}
		return nil
	})
}

// Dependencies initializes a new command instance for invocation
func (p *PutObjectSet) Dependencies(
	c base.Container,
) (err error) {
	p.db, err = c.DB()

	return err
}
