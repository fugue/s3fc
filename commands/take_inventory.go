package commands

import (
	"context"
	"encoding/json"
	"io"
	"s3fc/base"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// TakeInventory iterators over and s3 bucket prefix for object definitions and
// stores them as line delimeted json objects in either another s3 object or a
// file on disk
type TakeInventory struct {
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix"`
	Destination string `json:"destination"`

	logger    logrus.FieldLogger
	client    s3iface.S3API
	inventory base.InventoryManager
}

// Invoke triggers the TakeInventory command
func (t TakeInventory) Invoke(ctx context.Context) error {
	var wg sync.WaitGroup
	r, w := io.Pipe()

	t.logger.WithFields(logrus.Fields{
		"bucket":      t.Bucket,
		"prefix":      t.Prefix,
		"destination": t.Destination,
	}).Info("starting TakeInventory")

	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		enc := json.NewEncoder(w)
		err := t.client.ListObjectsV2PagesWithContext(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(t.Bucket),
			Prefix: aws.String(t.Prefix),
		}, func(o *s3.ListObjectsV2Output, b bool) bool {
			for _, ob := range o.Contents {
				err := enc.Encode(ob)
				if err != nil {
					t.logger.
						WithError(err).
						Error("error while json encoding list output")
					w.CloseWithError(err)
					return false
				}
			}

			return !b
		})

		if err != nil {
			t.logger.
				WithError(err).
				WithFields(logrus.Fields{
					"bucket": t.Bucket,
					"prefix": t.Prefix,
				}).
				Error("error while listing prefix")
		}

		w.CloseWithError(err)
	}()

	return t.inventory.WriteFrom(ctx, r, t.Destination)
}

// Dependencies initializes a new command instance for invocation
func (t *TakeInventory) Dependencies(
	c base.Container,
) (err error) {
	t.client, err = c.S3API()
	if err != nil {
		return err
	}
	t.logger = c.Logger()
	t.inventory = c.InventoryManager()

	return nil
}
