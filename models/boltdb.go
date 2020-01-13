package models

import (
	"bytes"
	"errors"
	"s3fc/boltdb"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

var (
	// ErrNotObjectSet values being unmarshaled are not for an object set
	ErrNotObjectSet = errors.New("Object is not an object set")
	// ErrNotSourceObject values being unmarshaled are not for a source object
	ErrNotSourceObject = errors.New("Object is not a source object")
	// ErrNotDestinationObject values being unmarshaled are not for a destination object
	ErrNotDestinationObject = errors.New("Object is not a destination object")

	valueTrue  = []byte{0x1}
	valueFalse = []byte{0x0}

	objectSchema = boltdb.Schema(
		"etag",
		"key",
		"storage_class",
		"last_modified",
		"owner_id",
		"owner_display_name",
		"size",
		"state",
	)
	objectIndexes = map[string][]byte{}

	sourceObjectSchema = updateMap(
		boltdb.Schema(
			"destination_object",
			"is_source_object",
		),
		objectSchema,
	)

	sourceObjectIndexes = updateMap(
		map[string][]byte{
			"destination_object": []byte("idx_destination"),
			"state":              []byte("idx_source_state"),
			"key":                []byte("idx_source_key"),
		},
		objectIndexes,
	)

	destinationObjectSchema = updateMap(
		boltdb.Schema(
			"is_destination_object",
		),
		objectSchema,
	)
	destinationObjectIndexes = updateMap(
		map[string][]byte{
			"state": []byte("idx_destination_state"),
			"key":   []byte("idx_destination_key"),
		},
		objectIndexes,
	)

	objectSetSchema = boltdb.Schema(
		"block_size",
		"destination_bucket",
		"destination_path",
		"delimiter",
	)

	objectSetColumns = mergeValues(
		updateMap(
			make(map[string][]byte),
			sourceObjectSchema,
			destinationObjectSchema,
		),
		sourceObjectIndexes,
		destinationObjectIndexes,
	)
)

// ObjectSetPrototype prototype function for instantiating an ObjectSet as a
// boltdb.Row using an anonymous function
func ObjectSetPrototype(bucket string, prefix string) func() boltdb.Row {
	return func() boltdb.Row {
		return NewObjectSet(bucket, prefix)
	}
}

// Name the partition/table name of an object set in a bolt database
func (o *ObjectSet) Name() []byte {
	return o.tableName
}

// Columns the column/sub-bucket list of an object set in a bolt database
func (o *ObjectSet) Columns() [][]byte {
	return objectSetColumns
}

// Schema the metadata keys of an object set in an bolt database
func (o *ObjectSet) Schema() map[string][]byte {
	return objectSetSchema
}

// Unmarshal maps values from of a bolt database to an object set
func (o *ObjectSet) Unmarshal(values map[string][]byte) error {
	if v, ok := values["block_size"]; ok {
		o.BlockSize = boltdb.Ltoi(v)
	}
	if v, ok := values["destination_bucket"]; ok {
		o.DestinationBucket = string(v)
	}
	if v, ok := values["destination_path"]; ok {
		o.DestinationPath = string(v)
	}
	if v, ok := values["delimiter"]; ok {
		o.Delimiter = v
	}

	return nil
}

// Marshal maps values from of an object set to a bolt database
func (o *ObjectSet) Marshal() (map[string][]byte, error) {
	return map[string][]byte{
		"block_size":         boltdb.Itol(o.BlockSize),
		"destination_bucket": []byte(o.DestinationBucket),
		"destination_path":   []byte(o.DestinationPath),
		"delimiter":          o.Delimiter,
	}, nil
}

// ObjectPrototype prototype function for instantiating an Object as a
// boltdb.Row using an anonymous function
func ObjectPrototype(p ObjectSet) func() boltdb.Row {
	return func() boltdb.Row {
		return NewObject(p)
	}
}

// PK returns the index column and primary key value of an Object for retrieving
// its database id
func (o *Object) PK() ([]byte, []byte) {
	return o.Indexes()["key"], []byte(aws.StringValue(o.Key))
}

// Schema map columns/buckets required for an Object type
func (o *Object) Schema() map[string][]byte {
	return objectSchema
}

// Indexes map of columns that are index by their value and database id
func (o *Object) Indexes() map[string][]byte {
	return objectIndexes
}

// Unmarshal maps values from of a bolt database to an Object
func (o *Object) Unmarshal(values map[string][]byte) error {

	if v, ok := values["etag"]; ok && v != nil {
		o.ETag = aws.String(string(v))
	} else {
		o.ETag = nil
	}

	if v, ok := values["key"]; ok && v != nil {
		o.Key = aws.String(string(v))
	} else {
		o.Key = nil
	}

	if v, ok := values["storage_class"]; ok && v != nil {
		o.StorageClass = aws.String(string(v))
	} else {
		o.StorageClass = nil
	}

	if v, ok := values["last_modified"]; ok && v != nil {
		o.LastModified = aws.Time(time.Unix(0, boltdb.Ltoi(v)))
	} else {
		o.LastModified = nil
	}

	owner := new(s3.Owner)
	o.Owner = nil
	if v, ok := values["owner_id"]; ok && v != nil {
		owner.ID = aws.String(string(v))
		o.Owner = owner
	}
	if v, ok := values["owner_display_name"]; ok && v != nil {
		owner.DisplayName = aws.String(string(v))
		o.Owner = owner
	}

	if v, ok := values["size"]; ok && v != nil {
		o.Size = aws.Int64(boltdb.Ltoi(v))
	} else {
		o.Size = nil
	}

	if v, ok := values["state"]; ok && v != nil {
		o.State = boltdb.Ltouint16(v)
	} else {
		o.State = StateUnknown
	}

	return nil
}

// Marshal maps values from an Object to a bolt database
func (o *Object) Marshal() (map[string][]byte, error) {
	values := map[string][]byte{
		"etag":               nil,
		"key":                nil,
		"storage_class":      nil,
		"last_modified":      nil,
		"owner_id":           nil,
		"owner_display_name": nil,
		"size":               nil,
		"state":              nil,
	}

	if o.ETag != nil {
		values["etag"] = []byte(aws.StringValue(o.ETag))
	}

	if o.Key != nil {
		_, values["key"] = o.PK()
	}

	if o.StorageClass != nil {
		values["storage_class"] = []byte(aws.StringValue(o.StorageClass))
	}

	if o.LastModified != nil {
		values["last_modified"] = boltdb.Itol(aws.TimeValue(o.LastModified).UnixNano())
	}

	if o.Owner != nil {
		if o.Owner.ID != nil {
			values["owner_id"] = []byte(aws.StringValue(o.Owner.ID))

		}
		if o.Owner.DisplayName != nil {
			values["owner_display_name"] = []byte(aws.StringValue(o.Owner.DisplayName))
		}
	}

	if o.Size != nil {
		values["size"] = boltdb.Itol(aws.Int64Value(o.Size))
	}

	values["state"] = boltdb.Uint16tol(o.State)

	return values, nil
}

// SourceObjectPrototype prototype function for instantiating a SourceObject as
// a boltdb.Row using an anonymous function
func SourceObjectPrototype(p ObjectSet) func() boltdb.Row {
	return func() boltdb.Row {
		return NewSourceObject(p)
	}
}

// PK returns the index column and primary key value of a SourceObject for
// retrieving its database id
func (s *SourceObject) PK() ([]byte, []byte) {
	return s.Indexes()["key"], []byte(aws.StringValue(s.Key)[len(s.Parent.Prefix):])
}

// Schema map columns/buckets required for a SourceObject type
func (s *SourceObject) Schema() map[string][]byte {
	return sourceObjectSchema
}

// Indexes map of columns that are index by their value and database id
func (s *SourceObject) Indexes() map[string][]byte {
	return sourceObjectIndexes
}

// Copy creates a copy of a SourceObject via marshaling and unmarshalling.
// This is useful for updates so that extra database look ups are required for
// managing indexes.
func (s *SourceObject) Copy() (*SourceObject, error) {
	cp := NewSourceObject(s.Parent)
	values, err := s.Marshal()
	if err != nil {
		return nil, err
	}
	err = cp.Unmarshal(values)
	if err != nil {
		return nil, err
	}

	return cp, nil
}

// Unmarshal maps values from of a bolt database to a SourceObject
func (s *SourceObject) Unmarshal(values map[string][]byte) error {
	if err := s.Object.Unmarshal(values); err != nil {
		return err
	}

	if v, ok := values["key"]; ok && v != nil {
		s.Key = aws.String(s.Parent.Prefix + string(v))
	} else {
		s.Key = nil
	}

	if v, ok := values["destination_object"]; ok {
		s.DestinationObjectID = v
	}

	if v, ok := values["is_source_object"]; !ok || !bytes.Equal(v, valueTrue) {
		return ErrNotDestinationObject
	}

	return nil
}

// Marshal maps values from of a SourceObject to a bolt database
func (s *SourceObject) Marshal() (map[string][]byte, error) {
	values, err := s.Object.Marshal()
	if err != nil {
		return nil, err
	}

	if s.Key != nil {
		_, values["key"] = s.PK()
	}

	values["destination_object"] = s.DestinationObjectID
	values["is_source_object"] = valueTrue

	return values, nil
}

// DestinationObjectPrototype prototype function for instantiating a
// DestinationObject as a boltdb.Row using an anonymous function
func DestinationObjectPrototype(p ObjectSet) func() boltdb.Row {
	return func() boltdb.Row {
		return NewDestinationObject(p)
	}
}

// PK returns the index column and primary key value of a DestinationObject for
// retrieving its database id
func (d *DestinationObject) PK() ([]byte, []byte) {
	return d.Indexes()["key"], []byte(aws.StringValue(d.Key)[len(d.Parent.DestinationPath):])
}

// Schema map columns/buckets required for a DestinationObject type
func (d *DestinationObject) Schema() map[string][]byte {
	return destinationObjectSchema
}

// Indexes map of columns that are index by their value and database id
func (d *DestinationObject) Indexes() map[string][]byte {
	return destinationObjectIndexes
}

// Copy creates a copy of a DestinationObject via marshaling and unmarshalling.
// This is useful for updates so that extra database look ups are required for
// managing indexes.
func (d *DestinationObject) Copy() (*DestinationObject, error) {
	cp := NewDestinationObject(d.Parent)
	values, err := d.Marshal()
	if err != nil {
		return nil, err
	}
	err = cp.Unmarshal(values)
	if err != nil {
		return nil, err
	}

	return cp, nil
}

// Unmarshal maps values from of a bolt database to a DestinationObject
func (d *DestinationObject) Unmarshal(values map[string][]byte) error {
	if err := d.Object.Unmarshal(values); err != nil {
		return err
	}

	if v, ok := values["key"]; ok && v != nil {
		d.Key = aws.String(d.Parent.DestinationPath + string(v))
	} else {
		d.Key = nil
	}

	if v, ok := values["is_destination_object"]; !ok || !bytes.Equal(v, valueTrue) {
		return ErrNotDestinationObject
	}

	return nil
}

// Marshal maps values from of a DestinationObject to a bolt database
func (d *DestinationObject) Marshal() (map[string][]byte, error) {
	values, err := d.Object.Marshal()
	if err != nil {
		return nil, err
	}

	if d.Key != nil {
		_, values["key"] = d.PK()
	}

	values["is_destination_object"] = valueTrue
	return values, nil
}

// updateMap override key values in a by a variable number of other maps
func updateMap(a map[string][]byte, b ...map[string][]byte) map[string][]byte {
	for _, i := range b {
		for k, v := range i {
			a[k] = v
		}
	}

	return a
}

func mergeValues(a ...map[string][]byte) [][]byte {
	cap := 0
	for _, p := range a {
		cap += len(p)
	}
	output := make([][]byte, 0, cap)
	for _, p := range a {
		for _, v := range p {
			output = append(output, v)
		}
	}

	return output
}
