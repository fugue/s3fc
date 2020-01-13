package models

import (
	"path"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	// StateUnknown used when an object is first instantiated
	StateUnknown uint16 = iota
	// StateNew used when an object is ready to be placed into a destination
	StateNew
	// StateDirty used when an object has been updated. If it has a destination,
	// it needs to be expired.
	StateDirty
	// StateInSync used when an object is either mapped to its destination or a
	// destination has been succesfully written.
	StateInSync
	// StateExpired and object is ready for deletion.
	StateExpired
	// StateDeleted and object has been deleted.
	StateDeleted

	stateUnknown = "UNKNOWN"
	stateNew     = "NEW"
	stateDirty   = "DIRTY"
	stateInSync  = "IN_SYNC"
	stateExpired = "EXPIRED"
	stateDeleted = "DELETED"
)

// State type wrapper for formatting uint16's as state strings
type State uint16

func (s State) String() string {
	switch uint16(s) {
	case StateUnknown:
		return stateUnknown
	case StateNew:
		return stateNew
	case StateDirty:
		return stateDirty
	case StateInSync:
		return stateInSync
	case StateExpired:
		return stateExpired
	case StateDeleted:
		return stateDeleted
	}

	return stateUnknown
}

// ParseState takes a string and returns its State unit16 value
func ParseState(s string) uint16 {
	switch s {
	case stateUnknown:
		return StateUnknown
	case stateNew:
		return StateNew
	case stateDirty:
		return StateDirty
	case stateInSync:
		return StateInSync
	case stateExpired:
		return StateExpired
	case stateDeleted:
		return StateDeleted
	}

	return StateUnknown
}

// ObjectSet a job/partition definition for managing the concatination of source
// objects to destination objects.
type ObjectSet struct {
	Bucket string
	Prefix string

	// Target configuration
	DestinationBucket string
	DestinationPath   string
	Delimiter         []byte
	BlockSize         int64

	tableName []byte
}

// NewObjectSet instantiates a new ObjectSet from its primary key values of
// bucket and prefix. It does not load other metadata such as destination/target
// configuration.
func NewObjectSet(bucket string, prefix string) *ObjectSet {
	return &ObjectSet{
		Bucket: bucket,
		Prefix: prefix,

		tableName: []byte(path.Join(bucket, prefix)),
	}
}

// Object base struct defining an s3 object that is a member of an object set.
type Object struct {
	Parent ObjectSet
	State  uint16
	s3.Object
}

// NewObject instantiates a new Object declaring it a member of the passed
// ObjectSet
func NewObject(p ObjectSet) *Object {
	return &Object{
		Parent: p,
		State:  StateUnknown,
	}
}

// IsDirty compares the receiver Object with the passed object. If their ETags's
// do not match the receiver is flag as dirty.
func (o *Object) IsDirty(other Object) bool {
	if aws.StringValue(o.ETag) == aws.StringValue(other.ETag) {
		return false
	}

	o.State = StateDirty
	return true
}

// SourceObject defines a s3 object that is flagged as a "source". This means
// it is an object that will be concatinated with other SourceObjects and written
// to a DestinationObject
type SourceObject struct {
	Object
	DestinationObjectID []byte
}

// NewSourceObject instantiates a new SourceObject declaring it a member of the
// passed ObjectSet
func NewSourceObject(p ObjectSet) *SourceObject {
	return &SourceObject{
		Object: Object{
			Parent: p,
			State:  StateUnknown,
		},
	}
}

// DestinationObject is an object that is the target of SourceObject
// concatination
type DestinationObject struct {
	Object
}

// NewDestinationObject instantiates a new DestinationObject declaring it a
// member of the passed ObjectSet
func NewDestinationObject(p ObjectSet) *DestinationObject {
	return &DestinationObject{
		Object: Object{
			Parent: p,
			State:  StateUnknown,
		},
	}
}
