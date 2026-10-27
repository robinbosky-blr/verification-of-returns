package worker

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/domain"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/storage"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestImportLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	schema := "test_" + store.ID()
	if _, e = p.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer p.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 2
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	db := &store.Store{Pool: pool}
	if e = db.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	files := storage.Local{Root: t.TempDir()}
	w := Worker{DB: db, Files: files}
	add := func(kind, name, content string) store.Import {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		os.WriteFile(path, []byte(content), 0600)
		id := store.ID()
		if e := files.Put(ctx, id, path); e != nil {
			t.Fatal(e)
		}
		i, dup, e := db.CreateImport(ctx, store.Import{ID: id, Kind: kind, Filename: name, Hash: id, Object: id})
		if e != nil || dup {
			t.Fatal(e)
		}
		if e = w.Once(ctx); e != nil {
			t.Fatal(e)
		}
		return i
	}
	web := add("website", "website.csv", "Order ID,SKU,Return Requested On,Last Updated On,Return Status\nA,S1,01/07/2026,08/10/2026,Returned to Seller\nB,S2,15/07/2026,20/07/2026,Return Initiated\n")
	add("payment", "payment.csv", "date/time,type,order id,sku,total\n03/10/2026,Order,A,S1,300\n03/10/2026,Refund,A,S1,-270\n05/10/2026,Fulfilment Fee Refund,A,S1,50\n")
	physical := add("physical", "physical.csv", "Order ID,SKU,Received Date\nB,S2,01/08/2026\n")
	result := func() domain.Summary {
		t.Helper()
		rs, e := db.Results(ctx, store.Filter{})
		if e != nil {
			t.Fatal(e)
		}
		return domain.Summarize(rs)
	}
	s := result()
	if s.Total != 2 || s.Received != 1 || s.Loss != 27000 {
		t.Fatal(s)
	}
	// Overlapping snapshots must not double a refund or create a new logical case.
	add("payment", "overlap.csv", "date/time,type,order id,sku,total\n03/10/2026,Order,A,S1,300\n03/10/2026,Refund,A,S1,-270\n05/10/2026,Fulfilment Fee Refund,A,S1,50\n06/10/2026,Order,B,S2,250\n")
	if s = result(); s.Loss != 27000 || s.Total != 2 {
		t.Fatal(s)
	}
	old, dup, e := db.CreateImport(ctx, store.Import{ID: store.ID(), Kind: web.Kind, Filename: web.Filename, Hash: web.Hash, Object: web.Object})
	if e != nil || !dup || old.ID != web.ID {
		t.Fatal(old, dup, e)
	}
	if e = db.SetActive(ctx, physical.ID, false); e != nil {
		t.Fatal(e)
	}
	if e = w.Once(ctx); e != nil {
		t.Fatal(e)
	}
	if s = result(); s.Received != 0 || s.NotReceived != 2 {
		t.Fatal(s)
	}
	if e = db.SetActive(ctx, physical.ID, true); e != nil {
		t.Fatal(e)
	}
	if e = w.Once(ctx); e != nil {
		t.Fatal(e)
	}
	if s = result(); s.Received != 1 {
		t.Fatal(s)
	}
	// Identical valid payment rows in one file remain distinct occurrences.
	add("payment", "twice.csv", "date/time,type,order id,sku,total\n03/10/2026,Refund,A,S1,-270\n03/10/2026,Refund,A,S1,-270\n")
	if s = result(); s.Loss != 54000 {
		t.Fatal(s)
	}
	// Failed file is atomic: one invalid row cannot partially change published data.
	bad := store.ID()
	path := filepath.Join(t.TempDir(), "bad.csv")
	os.WriteFile(path, []byte("Order ID,SKU,Received Date\nA,S1,03/07/2026\nB,S2,bad-date\n"), 0600)
	files.Put(ctx, bad, path)
	db.CreateImport(ctx, store.Import{ID: bad, Kind: "physical", Filename: "bad.csv", Hash: bad, Object: bad})
	if e = w.Once(ctx); e == nil {
		t.Fatal("expected validation error")
	}
	if s = result(); s.Loss != 54000 || s.Received != 1 {
		t.Fatal(s)
	}
	var count int
	if e = pool.QueryRow(ctx, "SELECT count(*) FROM import_rows WHERE import_id=$1", bad).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
