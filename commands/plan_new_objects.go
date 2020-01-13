package commands

import (
	"context"
	"path"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/uuid"

	"github.com/boltdb/bolt"
)

// PlanNewObjects queries for NEW source objects and builds out new destination
// objects. This only updates the model's state and does not actually
// concatinate the source objects to the destination objects
type PlanNewObjects struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`

	db *bolt.DB
}

// Invoke triggers the PlanNewObjects command
func (p PlanNewObjects) Invoke(ctx context.Context) error {
	set := models.NewObjectSet(p.Bucket, p.Prefix)

	if err := p.db.View(func(tx *bolt.Tx) error {
		_, err := boltdb.LookupTable(tx, set)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	index := []byte("idx_source_state")
	prefix := boltdb.Uint16tol(models.StateNew)
	limit := 2048

	var blockID []byte
	var block *models.DestinationObject
	var n int64
	var objectIds [][]byte
	var err error

	for {
		objectIds = nil

		if err := p.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(set.Name())
			ids, err := boltdb.PrefixQuery(
				b, index, prefix, limit, nil,
			)
			if err != nil {
				return err
			}

			objectIds = ids
			return nil
		}); err != nil {
			return err
		}

		if err := p.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(set.Name())
			for _, id := range objectIds {
				source := models.NewSourceObject(*set)
				if err = boltdb.LookupRow(b, id, source); err != nil {
					return err
				}

				if blockID == nil {
					n = 0
					block = models.NewDestinationObject(*set)
					block.Key = aws.String(path.Join(
						block.Parent.DestinationPath,
						uuid.Must(uuid.NewRandom()).String(),
					))
					block.State = models.StateNew

					blockID, err = boltdb.AppendRow(b, block)
					if err != nil {
						return err
					}
				}

				current, err := source.Copy()
				if err != nil {
					return err
				}
				source.DestinationObjectID = blockID
				source.State = models.StateInSync
				if err = boltdb.UpdateRow(b, id, source, current); err != nil {
					return err
				}

				n += aws.Int64Value(source.Size) + int64(len(set.Delimiter))
				if n >= set.BlockSize {
					if err = p.flushDestination(b, blockID, block, n); err != nil {
						return err
					}
					blockID = nil
					block = nil
					n = 0
				}
			}

			if len(objectIds) < limit && blockID != nil {
				if err = p.flushDestination(b, blockID, block, n); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return err
		}

		if len(objectIds) < limit {
			return nil
		}
	}
}

// Dependencies initializes a new command instance for invocation
func (p *PlanNewObjects) Dependencies(
	c base.Container,
) (err error) {
	p.db, err = c.DB()

	return err
}

func (p *PlanNewObjects) flushDestination(
	b *bolt.Bucket,
	blockID []byte,
	block *models.DestinationObject,
	n int64,
) error {
	newBlock, err := block.Copy()
	if err != nil {
		return err
	}
	newBlock.Size = aws.Int64(n)
	if err = boltdb.UpdateRow(b, blockID, newBlock, block); err != nil {
		return err
	}

	return nil
}
