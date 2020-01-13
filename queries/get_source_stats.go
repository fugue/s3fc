package queries

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/models"

	"github.com/boltdb/bolt"
)

// GetSourceStats is a query that will return some basic statics about the
// SourceObjects inside of an ObjectSet
type GetSourceStats struct {
	Bucket string
	Prefix string

	db *bolt.DB
}

// GetSourceStatsOutput the ouput of the query
type GetSourceStatsOutput struct {
	Count  int64          `json:"count"`
	Size   string         `json:"size"`
	States map[string]int `json:"states"`

	size int64
}

// Invoke executes the GetSourceStats query
func (g GetSourceStats) Invoke(ctx context.Context, w io.Writer) error {

	set := models.NewObjectSet(g.Bucket, g.Prefix)

	return g.db.View(func(tx *bolt.Tx) error {
		var output GetSourceStatsOutput

		b := tx.Bucket(set.Name())
		size := b.Bucket([]byte("size"))
		state := b.Bucket([]byte("state"))
		stateCounts := map[string]int{}

		c := b.Bucket([]byte("is_source_object")).Cursor()
		for id, _ := c.First(); id != nil; id, _ = c.Next() {

			currState := state.Get(id)
			if currState == nil {
				log.Fatal("state not set")
			}
			stateKey := models.State(boltdb.Ltouint16(currState)).String()
			if val, ok := stateCounts[stateKey]; ok {
				stateCounts[stateKey] = val + 1
			} else {
				stateCounts[stateKey] = 1
			}
			output.size += boltdb.Ltoi(size.Get(id))
			output.Count++
		}
		output.States = stateCounts
		output.Size = fmt.Sprintf("%.3f GB", float64(output.size)/float64(1024*1024*1024))

		p, _ := json.MarshalIndent(output, "", "\t")
		w.Write(p)
		// enc := json.NewEncoder(w)
		// enc.Encode(output)

		return nil
	})
}

// Dependencies initializes a new command instance for invocation
func (g *GetSourceStats) Dependencies(
	c base.Container,
) (err error) {
	g.db, err = c.DB()

	return err
}
