package base

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

type Container interface {
	DB() (*bolt.DB, error)
	InventoryManager() InventoryManager
	Logger() logrus.FieldLogger
	S3API() (s3iface.S3API, error)

	Close() error
}

type Dependent interface {
	Dependencies(Container) error
}

type Command interface {
	Invoke(context.Context) error
}

type Query interface {
	Invoke(context.Context, io.Writer) error
}

type InventoryManager interface {
	WriteFrom(context.Context, *io.PipeReader, string) error
	ReadTo(context.Context, *io.PipeWriter, string) error
}
