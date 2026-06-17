package objectstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/objectstore"
)

type fakeS3 struct {
	gotBucket string
	err       error
}

func (f *fakeS3) HeadBucket(
	_ context.Context, in *s3.HeadBucketInput, _ ...func(*s3.Options),
) (*s3.HeadBucketOutput, error) {
	f.gotBucket = *in.Bucket

	if f.err != nil {
		return nil, f.err
	}

	return &s3.HeadBucketOutput{}, nil
}

func TestPing_Success(t *testing.T) {
	t.Parallel()

	api := &fakeS3{}
	store := objectstore.NewWith(api, "paddock-eu")

	if err := store.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	if api.gotBucket != "paddock-eu" {
		t.Errorf("HeadBucket bucket = %q, want paddock-eu", api.gotBucket)
	}
}

func TestPing_Error(t *testing.T) {
	t.Parallel()

	store := objectstore.NewWith(&fakeS3{err: errors.New("no such bucket")}, "paddock-eu")

	if err := store.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error, got nil")
	}
}

func TestOpen_BuildsStore(t *testing.T) {
	t.Parallel()

	store := objectstore.Open(config.ObjectStore{
		Endpoint:     "http://localhost:9000",
		Region:       "eu-central-1",
		AccessKey:    "minioadmin",
		SecretKey:    "minioadmin",
		Bucket:       "paddock-eu",
		UsePathStyle: true,
	})

	if store.Bucket() != "paddock-eu" {
		t.Errorf("Bucket() = %q, want paddock-eu", store.Bucket())
	}
}
