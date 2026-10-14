package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSqlcSchemaColumnsMatchMigrations(t *testing.T) {
	ctx, store := accountTestStore(t)
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_]+) \(`)
	matches := pattern.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("sqlc schema has no tables")
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	schemaName := "schema_check_" + hex.EncodeToString(suffix[:])
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO `+schemaName); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(source)); err != nil {
		t.Fatalf("sqlc schema cannot be applied: %v", err)
	}
	for _, match := range matches {
		table := match[1]
		actual := schemaColumns(t, ctx, tx, "public", table)
		declared := schemaColumns(t, ctx, tx, schemaName, table)
		slices.Sort(actual)
		slices.Sort(declared)
		if !slices.Equal(actual, declared) {
			t.Errorf("%s schema drift\nmigrations: %v\nsqlc:      %v", table, actual, declared)
		}
	}
}

func schemaColumns(t *testing.T, ctx context.Context, tx pgx.Tx, schema, table string) []string {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT column_name,data_type,COALESCE(character_maximum_length,0),is_nullable
		FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position`, schema, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var name, dataType, nullable string
		var maxLength int
		if err := rows.Scan(&name, &dataType, &maxLength, &nullable); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, fmt.Sprintf("%s:%s:%d:%s", name, dataType, maxLength, nullable))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}
