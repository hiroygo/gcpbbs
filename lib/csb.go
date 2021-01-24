package lib

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"time"

	"cloud.google.com/go/storage"
)

type CloudStorageBucket interface {
	Upload(objName string, obj io.Reader) (string, error)
}

type GCSBucket struct {
	client *storage.Client
	name   string
}

func NewGCSBucket(c *storage.Client, name string) CloudStorageBucket {
	return &GCSBucket{client: c, name: name}
}

func (b *GCSBucket) Upload(objName string, obj io.Reader) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	w := b.client.Bucket(b.name).Object(objName).NewWriter(ctx)
	if _, err := io.Copy(w, obj); err != nil {
		return "", fmt.Errorf("Copy error, %v", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("Close error, %v", err)
	}

	url, err := getGCSURL(b.name, objName)
	if err != nil {
		return "", fmt.Errorf("getGCSURL error, %v", err)
	}

	return url, nil
}

func getGCSURL(bucket, object string) (string, error) {
	u, err := url.Parse("https://storage.googleapis.com")
	if err != nil {
		return "", fmt.Errorf("Parse error, %v", err)
	}

	u.Path = path.Join(u.Path, bucket, object)
	return u.String(), nil
}
