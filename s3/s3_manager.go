package s3

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// CreateObject creates a new object at the provided bucket/key and returns
// a writer to upload content
func CreateObject(
	ctx context.Context,
	client s3iface.S3API,
	bucket string,
	key string,
	r io.Reader,
) error {
	uploader := s3manager.NewUploaderWithClient(client)
	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	return err
}

// DownloadObject will write an s3 object to the provided writer using the
// s3manager
func DownloadObject(
	ctx context.Context,
	client s3iface.S3API,
	bucket string,
	key string,
	w io.WriterAt,
) error {
	downloader := s3manager.NewDownloaderWithClient(client)
	_, err := downloader.DownloadWithContext(ctx, w, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

// IsNotFound checks if an error is for a nonexistent object.
func IsNotFound(err error) bool {
	awsErr, ok := err.(awserr.Error)
	return ok && awsErr.Code() == "NoSuchKey"
}
