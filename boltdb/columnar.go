package boltdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/boltdb/bolt"
)

const idSize = 8

// Schema takes a list of column names and maps them to wrapped byte array
// values
func Schema(cols ...string) map[string][]byte {
	m := make(map[string][]byte)
	for _, c := range cols {
		m[c] = []byte(c)
	}
	return m
}

// Table a top level boltdb bucket
type Table interface {
	Name() []byte
	Columns() [][]byte
}

// Row an item in a Table
type Row interface {
	Unmarshal(map[string][]byte) error
	Marshal() (map[string][]byte, error)
	Schema() map[string][]byte
}

// Indexed declares the configuration of how index the implementer
type Indexed interface {
	Indexes() map[string][]byte
}

// PK returns the index and value of the implementers primary key so that it
// can be used to look up its bolt database id.
type PK interface {
	PK() ([]byte, []byte)
}

// EnsureTable creates a table and its columns.
// TODO: have it also write its metadata
func EnsureTable(db *bolt.DB, table Table) error {
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(table.Name())
		if err != nil {
			return err
		}
		for _, columnName := range table.Columns() {
			if _, err = b.CreateBucketIfNotExists(columnName); err != nil {
				return err
			}
		}

		return nil
	})
}

// LookupTable hydrates a tables metadata and returns its bolt database Bucket.
func LookupTable(
	tx *bolt.Tx,
	table Table,
) (*bolt.Bucket, error) {
	b := tx.Bucket(table.Name())

	if row, ok := table.(Row); ok {
		values := make(map[string][]byte)
		schema := row.Schema()
		for k, v := range schema {
			values[k] = b.Get(v)
		}

		if err := row.Unmarshal(values); err != nil {
			return nil, err
		}
	} else {
		fmt.Println("not a row")
	}

	return b, nil
}

// LookupRow hydrates a row by bolt database id.
func LookupRow(
	b *bolt.Bucket,
	id []byte,
	row Row,
) error {
	values := lookupRow(b, id, row)
	return row.Unmarshal(values)
}

func lookupRow(
	b *bolt.Bucket,
	id []byte,
	row Row,
) map[string][]byte {
	values := make(map[string][]byte)
	schema := row.Schema()
	for k, v := range schema {
		values[k] = b.Bucket(v).Get(id)
	}

	return values
}

// AppendRow adds a row to a bucket, processes its indexes, and returns its bolt
// database id.
func AppendRow(
	b *bolt.Bucket,
	row Row,
) ([]byte, error) {
	seq, err := b.NextSequence()
	if err != nil {
		return nil, err
	}

	id := make([]byte, idSize)
	binary.LittleEndian.PutUint64(id, seq)

	values, err := row.Marshal()
	if err != nil {
		return nil, err
	}

	var indexes map[string][]byte
	if idx, ok := row.(Indexed); ok {
		indexes = idx.Indexes()
	}

	schema := row.Schema()
	for k, v := range values {
		if v == nil {
			continue
		}

		name, ok := schema[k]
		col := b.Bucket(name)
		if !ok {
			log.Fatalf("Key not defined in schema: %s", k)
		}
		if col == nil {
			log.Fatalf("Bucket not found: %s", string(schema[k]))
		}

		if err = col.Put(id, v); err != nil {
			return nil, err
		}

		if indexes != nil {
			if index, ok := indexes[k]; ok {
				newIndex := MakeIndex(v, id)
				if err = b.Bucket(index).Put(newIndex, nil); err != nil {
					return nil, err
				}
			}
		}
	}

	return id, nil
}

// UpdateRow updates a rows data by bolt database id and processes any changes
// to its indexes. If current is passed as nil, the current value will be
// reteived.
func UpdateRow(
	b *bolt.Bucket,
	id []byte,
	row Row,
	current Row,
) error {
	newValues, err := row.Marshal()
	if err != nil {
		return err
	}

	var currValues map[string][]byte
	if current != nil {
		currValues, err = current.Marshal()
		if err != nil {
			return err
		}
	} else {

		currValues = lookupRow(b, id, row)
	}

	var indexes map[string][]byte
	if idx, ok := row.(Indexed); ok {
		indexes = idx.Indexes()
	}

	schema := row.Schema()
	for k, v := range newValues {
		if err = putValue(b, schema, indexes, id, k, v, currValues); err != nil {
			return err
		}
	}

	return nil
}

func putValue(
	b *bolt.Bucket,
	schema map[string][]byte,
	indexes map[string][]byte,
	id []byte,
	k string,
	v []byte,
	currValues map[string][]byte,
) error {
	var err error
	if bytes.Equal(v, currValues[k]) {
		return nil
	}

	if v == nil {
		if err = b.Bucket(schema[k]).Delete(id); err != nil {
			return err
		}
	} else if err = b.Bucket(schema[k]).Put(id, v); err != nil {
		return err
	}

	if indexes != nil {
		if index, ok := indexes[k]; ok {
			oldIndex := MakeIndex(currValues[k], id)
			if err = b.Bucket(index).Delete(oldIndex); err != nil {
				return err
			}

			if v == nil {
				return nil
			}

			newIndex := MakeIndex(v, id)
			if err = b.Bucket(index).Put(newIndex, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// PrefixQuery queries and index by prefix and returns up to `limit` bolt
// database ids. If the length of the returned set is equal to `limit`, the
// last id can be passed in as the `exclusiveStart` parameter to continue the
// query from where it left off.
func PrefixQuery(
	b *bolt.Bucket,
	index []byte,
	prefix []byte,
	limit int,
	exclusiveStart []byte,
) ([][]byte, error) {
	rows := make([][]byte, 0, limit)
	c := b.Bucket(index).Cursor()

	var idx []byte
	if exclusiveStart != nil {
		idx, _ = c.Seek(exclusiveStart)
		if idx == nil {
			return rows, nil
		}
		if bytes.Equal(idx, exclusiveStart) {
			idx, _ = c.Next()
		}
	} else {
		idx, _ = c.Seek(prefix)
	}

	for ; idx != nil && bytes.HasPrefix(idx, prefix); idx, _ = c.Next() {
		id := idFromIndex(idx)
		rows = append(rows, id)
		if len(rows) == limit {
			break
		}
	}
	return rows, nil
}

// LookupID retreives the bolt database id of a primary key.
func LookupID(b *bolt.Bucket, pk PK) ([]byte, error) {
	index, prefix := pk.PK()
	ids, err := PrefixQuery(b, index, prefix, 1, nil)
	if err != nil || len(ids) < 1 {
		return nil, err
	}

	return ids[0], nil
}

// MakeIndex creates the bolt database key for an index.
func MakeIndex(v []byte, id []byte) []byte {
	idx := make([]byte, len(v), len(v)+len(id))
	copy(idx, v)
	return append(idx, id...)
}

// idFromIndex gets the id out of an index key.
func idFromIndex(idx []byte) []byte {
	id := make([]byte, idSize)
	copy(id, idx[len(idx)-idSize:])
	return id
}

// Itol converts an int64 to a byte array in little endian order
func Itol(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// Ltoi converts a byte array in little endian order to an int64
func Ltoi(b []byte) int64 {
	return int64(binary.LittleEndian.Uint64(b))
}

// Uint16tol converts an uint16 to a byte array in little endian order
func Uint16tol(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

// Ltouint16 converts a byte array in little endian order to an uint16
func Ltouint16(b []byte) uint16 {
	return binary.LittleEndian.Uint16(b)
}
