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

type gcsBucket struct {
	client *storage.Client
	name   string
}

func OpenGCSBucket(c *storage.Client, name string) CloudStorageBucket {
	return &gcsBucket{client: c, name: name}
}

func (b *gcsBucket) Upload(objName string, obj io.Reader) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	w := b.client.Bucket(b.name).Object(objName).NewWriter(ctx)
	if _, err := io.Copy(w, obj); err != nil {
		return "", fmt.Errorf("Copy error, %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("Close error, %w", err)
	}

	url, err := getGCSURL(b.name, objName)
	if err != nil {
		return "", fmt.Errorf("getGCSURL error, %w", err)
	}

	return url, nil
}

func getGCSURL(bucket, object string) (string, error) {
	u, err := url.Parse("https://storage.googleapis.com")
	if err != nil {
		return "", fmt.Errorf("Parse error, %w", err)
	}

	u.Path = path.Join(u.Path, bucket, object)
	return u.String(), nil
}
