package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/boltdb/bolt"
)

// LoadInventory command that loads the inject db with source data from either
// a s3 object or a file on disk
type LoadInventory struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`

	Source *string `json:"source,omitempty"`

	db        *bolt.DB
	client    s3iface.S3API
	inventory base.InventoryManager
}

// Invoke triggers the LoadInventory command
func (l LoadInventory) Invoke(parent context.Context) error {
	sourceCtx, cancel := context.WithCancel(parent)
	defer cancel()

	r, err := l.resolveSource(sourceCtx)
	if err != nil {
		return err
	}

	objectSet := *models.NewObjectSet(l.Bucket, l.Prefix)
	buf := make([]s3.Object, 0, 2048)

	err = r.forEach(sourceCtx, func(ctx context.Context, o s3.Object) error {
		buf = append(buf, o)
		if len(buf) == cap(buf) {
			buf, err = l.flushBuffer(objectSet, buf)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	_, err = l.flushBuffer(objectSet, buf)
	return err
}

// Dependencies initializes a new command instance for invocation
func (l *LoadInventory) Dependencies(
	c base.Container,
) (err error) {
	l.client, err = c.S3API()
	if err != nil {
		return err
	}
	l.db, err = c.DB()
	l.inventory = c.InventoryManager()

	return err
}

func (l *LoadInventory) resolveSource(ctx context.Context) (*objectReader, error) {
	if l.Source == nil {
		// TODO: load directly from s3 list v2
		return nil, fmt.Errorf("Load via s3 ListObjectsV2 Not Implemented")
	}

	r, w := io.Pipe()
	go l.inventory.ReadTo(ctx, w, *l.Source)
	return &objectReader{r}, nil
}

func (l *LoadInventory) flushBuffer(
	objectSet models.ObjectSet,
	buf []s3.Object,
) ([]s3.Object, error) {
	if err := l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(objectSet.Name())
		if b == nil {
			return fmt.Errorf("Bucket not found: %s", string(objectSet.Name()))
		}
		for _, i := range buf {
			obj := models.NewSourceObject(objectSet)
			obj.Object.Object = i

			id, err := boltdb.LookupID(b, obj)
			if err != nil {
				return err
			}

			if id != nil {
				current := models.NewSourceObject(obj.Parent)
				if err = boltdb.LookupRow(b, id, current); err != nil {
					return err
				}

				if !obj.IsDirty(current.Object) {
					continue
				}

				if err = boltdb.UpdateRow(b, id, obj, current); err != nil {
					return err
				}
			} else {
				obj.State = models.StateNew
				if _, err = boltdb.AppendRow(b, obj); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return buf[0:0], nil
}

type objectReader struct {
	*io.PipeReader
}

type objectHandler func(context.Context, s3.Object) error

func (r *objectReader) forEach(
	ctx context.Context,
	f objectHandler,
) error {
	var o s3.Object

	dec := json.NewDecoder(r)
	err := dec.Decode(&o)
	for ; err == nil; err = dec.Decode(&o) {

		if nestedErr := f(ctx, o); nestedErr != nil {
			r.CloseWithError(nestedErr)
			return nestedErr
		}
		o.ETag = nil
		o.Key = nil
		o.LastModified = nil
		o.Owner = nil
		o.Size = nil
		o.StorageClass = nil
	}

	if err == io.EOF {
		err = nil
	}

	r.CloseWithError(err)
	return err
}
