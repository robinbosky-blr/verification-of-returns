package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSupabaseLifecycle(t *testing.T) {
	const content = "website return data"
	var stored string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("apikey") != "test-secret" {
			http.Error(w, "missing server credentials", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/returns-originals/amazon/import.csv":
			body, _ := io.ReadAll(r.Body)
			stored = string(body)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/object/authenticated/returns-originals/amazon/import.csv":
			_, _ = io.WriteString(w, stored)
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/v1/object/returns-originals":
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"prefixes":["amazon/import.csv"]}` {
				t.Fatalf("unexpected delete payload: %s", body)
			}
			stored = ""
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := Supabase{URL: server.URL, Key: "test-secret", Bucket: "returns-originals", Client: server.Client()}
	source := filepath.Join(t.TempDir(), "source.csv")
	if err := os.WriteFile(source, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "amazon/import.csv", source); err != nil {
		t.Fatal(err)
	}
	materialized, cleanup, err := store.Materialize(context.Background(), "amazon/import.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := os.ReadFile(materialized)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("materialized content = %q", got)
	}
	if err := store.Delete(context.Background(), "amazon/import.csv"); err != nil {
		t.Fatal(err)
	}
}

func TestSupabaseRejectsUnsafeKeys(t *testing.T) {
	store := Supabase{}
	for _, key := range []string{"", "../secret", "/absolute", "folder//file"} {
		if err := store.Delete(context.Background(), key); err == nil {
			t.Fatalf("expected %q to be rejected", key)
		}
	}
}
