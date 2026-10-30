package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/domain"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/imports"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/storage"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/store"
	"log/slog"
	"strings"
	"time"
)

type Worker struct {
	DB    *store.Store
	Files storage.Store
}

func rowFingerprint(r domain.Row) string {
	canonical := r
	canonical.Source = ""
	canonical.Line = 0
	canonical.Raw = nil
	canonical.ImportedAt = time.Time{}
	canonical.Status = domain.Normalize(canonical.Status)
	canonical.Type = domain.Normalize(canonical.Type)
	canonical.Channel = domain.Normalize(canonical.Channel)
	var payload []byte
	if r.Kind == "payment" && r.ReferenceID != "" {
		payload = []byte(strings.Join([]string{"payment", domain.Normalize(r.ReferenceID), domain.Normalize(r.OrderID), domain.Normalize(r.SKU), domain.Normalize(r.Type), domain.Day(r.Date).Format("2006-01-02")}, "\x1f"))
	} else {
		payload, _ = json.Marshal(canonical)
	}
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

func (w Worker) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if e := w.Once(ctx); e != nil {
			slog.Error("job", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (w Worker) Once(ctx context.Context) error {
	tx, e := w.DB.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var locked bool
	if e = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(839104)").Scan(&locked); e != nil || !locked {
		return e
	}
	var job int64
	var id *string
	var kind string
	var marketplace string
	e = tx.QueryRow(ctx, "SELECT id,import_id,kind,marketplace FROM jobs WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&job, &id, &kind, &marketplace)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE jobs SET status='processing',attempts=attempts+1 WHERE id=$1", job); e != nil {
		return e
	}
	e = w.process(ctx, tx, id, kind, marketplace)

	if e != nil {

		tx.Rollback(ctx)
		errctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		fail, err := w.DB.Pool.Begin(errctx)
		if err != nil {
			return err
		}
		defer fail.Rollback(errctx)
		if _, err = fail.Exec(errctx, "UPDATE jobs SET status='failed',error=$2,attempts=attempts+1,finished_at=now() WHERE id=$1", job, e.Error()); err != nil {
			return err
		}
		if id != nil && kind == "import" {
			if _, err = fail.Exec(errctx, "UPDATE imports SET status='failed',error=$3 WHERE id=$1 AND marketplace=$2", *id, marketplace, e.Error()); err != nil {
				return err
			}
		}
		if err = store.AuditMarketplace(errctx, fail, marketplace, "job.failed", map[string]any{"job": job, "error": e.Error()}); err != nil {
			return err
		}
		if err = fail.Commit(errctx); err != nil {
			return err
		}
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE jobs SET status='completed',finished_at=now() WHERE id=$1", job); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (w Worker) process(ctx context.Context, tx pgx.Tx, id *string, kind, marketplace string) error {
	if id != nil && kind == "import" {
		var filename, key, typ, month string
		if e := tx.QueryRow(ctx, "SELECT filename,object_key,kind,coverage_month FROM imports WHERE id=$1 AND marketplace=$2", *id, marketplace).Scan(&filename, &key, &typ, &month); e != nil {
			return e
		}
		path, cleanup, e := w.Files.Materialize(ctx, key)
		if e != nil {
			return e
		}
		defer cleanup()
		counts := map[string]int{}
		batch := make([][]any, 0, 1000)
		total := 0
		sourceRows := 0
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			_, e := tx.CopyFrom(ctx, pgx.Identifier{"import_rows"}, []string{"import_id", "marketplace", "kind", "fingerprint", "occurrence", "order_id", "sku", "row_date", "payload"}, pgx.CopyFromRows(batch))
			batch = batch[:0]
			return e
		}
		_, e = imports.Parse(path, imports.Options{Marketplace: marketplace, Kind: typ, Filename: filename, Month: month}, func(r domain.Row) error {
			sourceRows++
			if sourceRows > 2000000 {
				return fmt.Errorf("import exceeds 2 million rows; split the file")
			}
			fp := rowFingerprint(r)
			counts[fp]++
			occ := 1
			if typ == "payment" {
				occ = counts[fp]
			} else if counts[fp] > 1 {
				return nil
			}
			r.Source = filename + " / " + r.Source
			b, e := json.Marshal(r)
			if e != nil {
				return e
			}
			batch = append(batch, []any{*id, marketplace, typ, fp, occ, r.OrderID, r.SKU, domain.Day(r.Date), b})
			total++
			if len(batch) == 1000 {
				return flush()
			}
			return nil
		})
		if e != nil {
			return e
		}
		if e = flush(); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE imports SET status='completed',rows_count=$3,completed_at=now() WHERE id=$1 AND marketplace=$2", *id, marketplace, sourceRows); e != nil {
			return e
		}
	}
	// Recompute affected orders only, including every SKU to protect order-level allocation.
	var orders []string
	if id != nil {
		rr, e := tx.Query(ctx, "SELECT DISTINCT order_id FROM import_rows WHERE import_id=$1 AND marketplace=$2", *id, marketplace)
		if e != nil {
			return e
		}
		for rr.Next() {
			var v string
			if e = rr.Scan(&v); e != nil {
				rr.Close()
				return e
			}
			orders = append(orders, v)
		}
		e = rr.Err()
		rr.Close()
		if e != nil {
			return e
		}
		if len(orders) == 0 {
			return nil
		}
	}
	rows, e := tx.Query(ctx, `SELECT DISTINCT ON (r.kind,r.fingerprint,r.occurrence) r.payload,i.created_at FROM import_rows r JOIN imports i ON i.id=r.import_id AND i.marketplace=r.marketplace WHERE r.marketplace=$1 AND i.status='completed' AND i.active AND ($2::text[] IS NULL OR r.order_id=ANY($2)) ORDER BY r.kind,r.fingerprint,r.occurrence,i.created_at DESC`, marketplace, orders)
	if e != nil {
		return e
	}
	data := []domain.Row{}
	for rows.Next() {
		var b []byte
		var importedAt time.Time
		if e = rows.Scan(&b, &importedAt); e != nil {
			rows.Close()
			return e
		}
		var r domain.Row
		if e = json.Unmarshal(b, &r); e != nil {
			rows.Close()
			return e
		}
		r.ImportedAt = importedAt
		data = append(data, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	results, e := domain.ReconcileMarketplace(marketplace, data, time.Now())
	if e != nil {
		return e
	}
	summary := domain.Summarize(results)
	sb, e := json.Marshal(summary)
	if e != nil {
		return e
	}
	var run int64
	ruleVersion := domain.RuleVersion
	if marketplace == "flipkart" { ruleVersion = domain.FlipkartRuleVersion }
	if marketplace == "myntra" { ruleVersion = domain.MyntraRuleVersion }
	if e = tx.QueryRow(ctx, "INSERT INTO reconciliation_runs(marketplace,rule_version,status,summary,completed_at) VALUES($1,$2,'completed',$3,now()) RETURNING id", marketplace, ruleVersion, string(sb)).Scan(&run); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM results WHERE marketplace=$1 AND ($2::text[] IS NULL OR order_id=ANY($2))", marketplace, orders); e != nil {
		return e
	}
	batch := make([][]any, 0, len(results))
	for _, r := range results {
		b, e := json.Marshal(r)
		if e != nil {
			return e
		}
		batch = append(batch, []any{marketplace, r.Key, r.OrderID, r.SKU, r.Month, r.Receipt, r.Financial, run, b})
	}
	if len(batch) > 0 {
		if _, e = tx.CopyFrom(ctx, pgx.Identifier{"results"}, []string{"marketplace", "case_key", "order_id", "sku", "month", "receipt", "financial", "run_id", "payload"}, pgx.CopyFromRows(batch)); e != nil {
			return e
		}
	}
	return store.AuditMarketplace(ctx, tx, marketplace, "reconciliation.completed", map[string]any{"run_id": run, "rule_version": ruleVersion, "affected_orders": len(orders), "summary": summary})
}
