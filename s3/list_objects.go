package s3

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

type objectLister struct {
	ctx      context.Context
	objectCh chan *s3.Object
}

func (o *objectLister) pageHandler(
	output *s3.ListObjectsV2Output,
	lastPage bool,
) bool {
	for _, object := range output.Contents {
		select {
		case <-o.ctx.Done():
			return false
		case o.objectCh <- object:
		}
	}
	return !lastPage
}

// ListObjects does an asynchronous call to ListObjectsV2PagesWithContext and
// sends the results by item to the returned channel.
func ListObjects(
	ctx context.Context,
	errCh chan error,
	client s3iface.S3API,
	input *s3.ListObjectsV2Input,
) chan *s3.Object {
	var o objectLister
	o.ctx = ctx
	o.objectCh = make(chan *s3.Object)

	go func(o *objectLister) {
		defer func() {
			close(o.objectCh)
			log.Printf("lister: done listing items")
		}()
		log.Printf("lister: starting to list items")
		if err := client.ListObjectsV2PagesWithContext(
			ctx, input, o.pageHandler,
		); err != nil {
			select {
			case <-o.ctx.Done():
				log.Printf("lister: problem listing objects: %v", err)
				return
			case errCh <- err:
			}
		}
	}(&o)

	return o.objectCh
}
