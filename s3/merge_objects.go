package s3

import (
	"context"
	"io"
	"s3fc/models"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// MergeObjects writes the provided list of SourceObjects to an S3 Object as per
// the configuration of the passed DestinationObject.
func MergeObjects(
	ctx context.Context,
	client s3iface.S3API,
	destination models.DestinationObject,
	sourceObjects []models.SourceObject,
) (int64, error) {
	uploader := s3manager.NewUploaderWithClient(client)

	r, w := io.Pipe()
	nCh := make(chan int64)
	go func() {
		defer close(nCh)
		var n int64
		for _, source := range sourceObjects {
			output, err := client.GetObjectWithContext(ctx, &s3.GetObjectInput{
				Bucket: aws.String(source.Parent.Bucket),
				Key:    source.Key,
			})
			if err != nil {
				w.CloseWithError(err)
				return
			}

			written, err := io.Copy(w, output.Body)
			output.Body.Close()

			if err != nil {
				w.CloseWithError(err)
				return
			}

			dWritten, err := w.Write(destination.Parent.Delimiter)
			if err != nil {
				w.CloseWithError(err)
				return
			}

			n += written + int64(dWritten)
		}
		w.Close()
		nCh <- n
	}()

	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(destination.Parent.Bucket),
		Key:    destination.Key,
		Body:   r,
	})
	if err != nil {
		r.CloseWithError(err)
		return <-nCh, err
	}

	return <-nCh, nil
}
