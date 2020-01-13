package inventory

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"s3fc/s3"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/sirupsen/logrus"
)

type InventoryManager struct {
	client s3iface.S3API
	logger logrus.FieldLogger
}

func New(
	client s3iface.S3API,
	logger logrus.FieldLogger,
) *InventoryManager {
	return &InventoryManager{
		client: client,
		logger: logger,
	}
}

func (i *InventoryManager) WriteFrom(
	ctx context.Context,
	r *io.PipeReader,
	destination string,
) error {
	destinationURL, err := url.Parse(destination)
	if err != nil {
		r.CloseWithError(err)
		return err
	}

	i.logger.WithFields(logrus.Fields{
		"scheme": destinationURL.Scheme,
		"host":   destinationURL.Host,
		"path":   destinationURL.Path,
	}).Info("starting upload")

	switch destinationURL.Scheme {
	case "file":
		return fileDestination(destinationURL, r)
	case "s3":
		return s3Destination(ctx, i.client, destinationURL, r)
	}

	r.CloseWithError(err)
	return fmt.Errorf("Invalid Destination: %s", destination)
}

func (i *InventoryManager) ReadTo(
	ctx context.Context,
	w *io.PipeWriter,
	source string,
) (err error) {
	defer func() {
		w.CloseWithError(err)
	}()

	var sourceURL *url.URL
	sourceURL, err = url.Parse(source)
	if err != nil {
		return err
	}

	switch sourceURL.Scheme {
	case "file":
		return fileSource(sourceURL, w)
	case "s3":
		return s3Source(ctx, i.client, sourceURL, w)
	default:
		return fmt.Errorf("Invalid Source: %s", sourceURL.Scheme)
	}
}

func fileDestination(
	url *url.URL,
	r *io.PipeReader,
) error {
	f, err := os.Create(url.Path)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, r)
	r.CloseWithError(err)
	f.Close()

	return err
}

func s3Destination(
	ctx context.Context,
	client s3iface.S3API,
	url *url.URL,
	r *io.PipeReader,
) error {
	bucket := url.Hostname()
	key := strings.Trim(url.EscapedPath(), "/")
	err := s3.CreateObject(ctx, client, bucket, key, r)
	r.CloseWithError(err)
	return err
}

func fileSource(url *url.URL, w *io.PipeWriter) error {
	file, err := os.Open(url.Path)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, file)
	file.Close()
	return err
}

func s3Source(
	ctx context.Context,
	client s3iface.S3API,
	url *url.URL,
	w *io.PipeWriter,
) error {
	bucket := url.Hostname()
	key := strings.Trim(url.EscapedPath(), "/")

	output, err := client.GetObjectWithContext(ctx, &awsS3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, output.Body)
	output.Body.Close()
	return err
}
