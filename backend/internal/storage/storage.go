package storage

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Store interface {
	Put(context.Context, string, string) error
	Materialize(context.Context, string) (string, func(), error)
	Delete(context.Context, string) error
}
type Local struct{ Root string }

func (s Local) path(k string) (string, error) {
	if strings.Contains(k, "..") || filepath.IsAbs(k) {
		return "", fmt.Errorf("invalid storage key")
	}
	return filepath.Join(s.Root, k), nil
}
func (s Local) Put(ctx context.Context, k, source string) error {
	p, e := s.path(k)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	if _, e = os.Stat(p); e == nil {
		return nil
	}
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	f, e := os.CreateTemp(filepath.Dir(p), ".upload-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = io.Copy(f, in)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}
func (s Local) Materialize(ctx context.Context, k string) (string, func(), error) {
	p, e := s.path(k)
	return p, func() {}, e
}
func (s Local) Delete(ctx context.Context, k string) error {
	p, e := s.path(k)
	if e != nil {
		return e
	}
	e = os.Remove(p)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}

type S3 struct {
	Client *minio.Client
	Bucket string
}

func New() (Store, error) {
	if ep := os.Getenv("S3_ENDPOINT"); ep != "" {
		c, e := minio.New(ep, &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), ""), Secure: os.Getenv("S3_SECURE") != "false"})
		if e != nil {
			return nil, e
		}
		return S3{c, os.Getenv("S3_BUCKET")}, nil
	}
	root := os.Getenv("DATA_DIR")
	if root == "" {
		root = "./data"
	}
	return Local{root}, os.MkdirAll(root, 0700)
}
func (s S3) Put(ctx context.Context, k, p string) error {
	_, e := s.Client.FPutObject(ctx, s.Bucket, k, p, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	return e
}
func (s S3) Materialize(ctx context.Context, k string) (string, func(), error) {
	f, e := os.CreateTemp("", "returns-object-*")
	if e != nil {
		return "", nil, e
	}
	p := f.Name()
	f.Close()
	e = s.Client.FGetObject(ctx, s.Bucket, k, p, minio.GetObjectOptions{})
	return p, func() { os.Remove(p) }, e
}
func (s S3) Delete(ctx context.Context, k string) error {
	return s.Client.RemoveObject(ctx, s.Bucket, k, minio.RemoveObjectOptions{})
}
