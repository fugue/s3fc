package queries

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/boltdb/bolt"
)

// ListObjectByState is a query that returns paginated response of Object IDs
// filtered by type ("source" or "destination") and state.
type ListObjectByState struct {
	Bucket         string  `json:"bucket"`
	Prefix         string  `json:"prefix"`
	Type           string  `json:"type"`
	State          string  `json:"state"`
	Limit          int     `json:"limit"`
	ExclusiveStart *string `json:"exclusive_start"`

	db *bolt.DB
}

// ListObjectByStateItem id and state of a found object
type ListObjectByStateItem struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Size  int64  `json:"size"`
}

// ListObjectByStateOutput the ouput of the query
type ListObjectByStateOutput struct {
	Type     string                  `json:"type"`
	Items    []ListObjectByStateItem `json:"items"`
	Length   int                     `json:"length"`
	NextPage *string                 `json:"next_page"`
}

// Invoke executes the ListObjectByState query
func (l ListObjectByState) Invoke(ctx context.Context, w io.Writer) error {

	return l.db.View(func(tx *bolt.Tx) error {
		var err error
		var exclusiveStart []byte

		l.Type = strings.ToLower(l.Type)
		l.State = strings.ToUpper(l.State)

		set := models.NewObjectSet(l.Bucket, l.Prefix)
		b, err := boltdb.LookupTable(tx, set)
		if err != nil {
			return err
		}
		index := []byte(fmt.Sprintf("idx_%s_state", l.Type))
		prefix := boltdb.Uint16tol(models.ParseState(l.State))

		if l.ExclusiveStart != nil {
			exclusiveStart, err = base64.RawURLEncoding.DecodeString(*l.ExclusiveStart)
			if err != nil {
				return err
			}
			exclusiveStart = boltdb.MakeIndex(prefix, exclusiveStart)
		}
		ids, err := boltdb.PrefixQuery(
			b, index, prefix, l.Limit, exclusiveStart,
		)
		if err != nil {
			return err
		}

		var lastID []byte
		items := make([]ListObjectByStateItem, 0, len(ids))
		for _, id := range ids {
			var row boltdb.Row
			var object *models.Object

			if l.Type == "source" {
				row = models.NewSourceObject(*set)
				object = &row.(*models.SourceObject).Object
			} else {
				row = models.NewDestinationObject(*set)
				object = &row.(*models.DestinationObject).Object
			}
			if err = boltdb.LookupRow(b, id, row); err != nil {
				return err
			}

			items = append(items, ListObjectByStateItem{
				ID: base64.RawURLEncoding.EncodeToString(id),
				// Bucket: set.Bucket,
				// Key:    aws.StringValue(object.Key),
				State: models.State(object.State).String(),
				Size:  aws.Int64Value(object.Size),
			})

			lastID = id
		}

		output := &ListObjectByStateOutput{
			Type:   l.Type,
			Items:  items,
			Length: len(items),
		}

		if lastID != nil && output.Length >= l.Limit {
			output.NextPage = aws.String(base64.RawURLEncoding.EncodeToString(lastID))
		}

		enc := json.NewEncoder(w)
		enc.Encode(output)

		return nil
	})
}

// Dependencies initializes a new command instance for invocation
func (l *ListObjectByState) Dependencies(
	c base.Container,
) (err error) {
	l.db, err = c.DB()

	return err
}
