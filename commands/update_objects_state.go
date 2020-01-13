package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"
	"strings"

	"github.com/boltdb/bolt"
)

// UpdateObjectsState is a command that will set the passed State to the
// provided list of bolt db ids in the passed ObjectSet by type ("destination"
// or "source").
type UpdateObjectsState struct {
	Bucket string   `json:"bucket"`
	Prefix string   `json:"prefix"`
	Type   string   `json:"type"`
	IDS    []string `json:"ids"`
	State  string   `json:"state"`

	db *bolt.DB
}

// Invoke triggers the UpdateObjectsState command
func (u UpdateObjectsState) Invoke(ctx context.Context) error {
	return u.db.Update(func(tx *bolt.Tx) error {
		state := models.ParseState(strings.ToUpper(u.State))
		if state == models.StateUnknown {
			return fmt.Errorf("Invalid state: %s", u.State)
		}
		set := models.NewObjectSet(u.Bucket, u.Prefix)
		b, err := boltdb.LookupTable(tx, set)
		if err != nil {
			return err
		}

		var current, row boltdb.Row
		for _, idB64 := range u.IDS {
			current, row = nil, nil
			id, err := base64.RawURLEncoding.DecodeString(idB64)
			if err != nil {
				return err
			}
			switch u.Type {
			case "source":
				o := models.NewSourceObject(*set)
				if err = boltdb.LookupRow(b, id, o); err != nil {
					return err
				}
				if o.State == state {
					continue
				}
				newO, err := o.Copy()
				if err != nil {
					return err
				}
				newO.State = state
				current, row = o, newO
			case "destination":
				o := models.NewDestinationObject(*set)
				if err = boltdb.LookupRow(b, id, o); err != nil {
					return err
				}
				if o.State == state {
					continue
				}
				newO, err := o.Copy()
				if err != nil {
					return err
				}
				newO.State = state
				current, row = o, newO
			}

			if err = boltdb.UpdateRow(b, id, row, current); err != nil {
				return err
			}
		}
		return nil
	})
}

// Dependencies initializes a new command instance for invocation
func (u *UpdateObjectsState) Dependencies(
	c base.Container,
) (err error) {
	u.db, err = c.DB()

	return err
}
