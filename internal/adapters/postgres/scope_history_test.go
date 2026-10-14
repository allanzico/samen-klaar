package postgres

import (
	"crypto/rand"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestScopeHistoryPersistsNewVersionsWithoutDuplicatingOldOnes(t *testing.T) {
	ctx, store := accountTestStore(t)
	id := "scope-" + rand.Text()
	t.Cleanup(func() { _, _ = store.pool.Exec(ctx, `DELETE FROM scope_versions WHERE job_id=$1`, id) })
	job := workflow.Job{ID: id, Quote: workflow.Quote{Currency: "EUR"}}
	for version := 1; version <= 4; version++ {
		job.ScopeHistory = append(job.ScopeHistory, workflow.ScopeSnapshot{Version: version, TotalMinor: int64(version * 1000), Lines: []workflow.QuoteLine{{Label: "Work", AmountMinor: int64(version * 1000)}}})
	}
	for pass := 0; pass < 2; pass++ {
		tx, err := store.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := persistScopeHistory(ctx, tx, job); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	job.ScopeHistory = append(job.ScopeHistory, workflow.ScopeSnapshot{Version: 5, TotalMinor: 5000, Lines: []workflow.QuoteLine{{Label: "Work", AmountMinor: 5000}}})
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistScopeHistory(ctx, tx, job); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM scope_versions WHERE job_id=$1`, id).Scan(&count); err != nil || count != 5 {
		t.Fatalf("recorded versions=%d, %v", count, err)
	}
}
