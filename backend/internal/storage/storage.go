package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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

// Supabase stores original imports in a private Storage bucket. The secret key
// is used only by the trusted API and worker and must never be sent to the web UI.
type Supabase struct {
	URL    string
	Key    string
	Bucket string
	Client *http.Client
}

func objectPath(key string) (string, error) {
	if strings.Contains(key, "..") || strings.HasPrefix(key, "/") || key == "" {
		return "", fmt.Errorf("invalid storage key")
	}
	parts := strings.Split(key, "/")
	for i := range parts {
		if parts[i] == "" {
			return "", fmt.Errorf("invalid storage key")
		}
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/"), nil
}

func (s Supabase) request(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.URL, "/")+endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Key)
	req.Header.Set("apikey", s.Key)
	if method == http.MethodDelete {
		req.Header.Set("Content-Type", "application/json")
	} else if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	return s.Client.Do(req)
}

func storageError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("supabase storage: %s: %s", resp.Status, strings.TrimSpace(string(message)))
}

func New() (Store, error) {
	if projectURL := os.Getenv("SUPABASE_URL"); projectURL != "" {
		key := os.Getenv("SUPABASE_SECRET_KEY")
		bucket := os.Getenv("SUPABASE_STORAGE_BUCKET")
		if key == "" || bucket == "" {
			return nil, fmt.Errorf("SUPABASE_SECRET_KEY and SUPABASE_STORAGE_BUCKET are required when SUPABASE_URL is set")
		}
		return Supabase{URL: projectURL, Key: key, Bucket: bucket, Client: &http.Client{Timeout: 5 * time.Minute}}, nil
	}
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

func (s Supabase) Put(ctx context.Context, key, source string) error {
	object, err := objectPath(key)
	if err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	endpoint := "/storage/v1/object/" + url.PathEscape(s.Bucket) + "/" + object
	resp, err := s.request(ctx, http.MethodPost, endpoint, file)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return storageError(resp)
}

func (s Supabase) Materialize(ctx context.Context, key string) (string, func(), error) {
	object, err := objectPath(key)
	if err != nil {
		return "", nil, err
	}
	endpoint := "/storage/v1/object/authenticated/" + url.PathEscape(s.Bucket) + "/" + object
	resp, err := s.request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if err = storageError(resp); err != nil {
		return "", nil, err
	}
	file, err := os.CreateTemp("", "returns-supabase-*")
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err = io.Copy(file, resp.Body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func (s Supabase) Delete(ctx context.Context, key string) error {
	if _, err := objectPath(key); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string][]string{"prefixes": []string{key}})
	if err != nil {
		return err
	}
	endpoint := "/storage/v1/object/" + url.PathEscape(s.Bucket)
	resp, err := s.request(ctx, http.MethodDelete, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return storageError(resp)
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
