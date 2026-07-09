package databaseintegration

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

type QueryResult struct {
	Columns   []string         `json:"columns,omitempty"`
	Rows      []map[string]any `json:"rows,omitempty"`
	RowCount  int              `json:"row_count"`
	Truncated bool             `json:"truncated"`
}

type ColumnInfo struct {
	Schema   string `json:"schema,omitempty"`
	Table    string `json:"table"`
	Column   string `json:"column"`
	DataType string `json:"data_type"`
}

func ExecuteSQL(ctx context.Context, provider, dsn, query string, policy Policy) (QueryResult, error) {
	prepared, err := PrepareSQL(provider, query, policy)
	if err != nil {
		return QueryResult{}, err
	}
	db, err := openSQL(provider, dsn)
	if err != nil {
		return QueryResult{}, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, prepared)
	if err != nil {
		return QueryResult{}, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()
	return readRows(rows, policy)
}

func TestSQL(ctx context.Context, provider, dsn string) error {
	db, err := openSQL(provider, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PingContext(ctx)
}

func IntrospectSQL(ctx context.Context, provider, dsn string) ([]ColumnInfo, error) {
	db, err := openSQL(provider, dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query := postgresSchemaQuery
	if provider == ProviderMySQL {
		query = mysqlSchemaQuery
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("introspect schema: %w", err)
	}
	defer rows.Close()
	var out []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		if err := rows.Scan(&c.Schema, &c.Table, &c.Column, &c.DataType); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func openSQL(provider, dsn string) (*sql.DB, error) {
	driver := provider
	if provider == ProviderPostgres {
		driver = "postgres"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	return db, nil
}

func readRows(rows *sql.Rows, policy Policy) (QueryResult, error) {
	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{}, err
	}
	limit := policy.MaxRows
	if limit <= 0 || limit > 1000 {
		limit = DefaultMaxRows
	}
	result := QueryResult{Columns: cols, Rows: []map[string]any{}}
	for rows.Next() {
		if result.RowCount >= limit {
			result.Truncated = true
			break
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return QueryResult{}, err
		}
		row := map[string]any{}
		for i, col := range cols {
			row[col] = normalizeSQLValue(values[i])
		}
		MaskRow(row, policy)
		result.Rows = append(result.Rows, row)
		result.RowCount++
	}
	return result, rows.Err()
}

func normalizeSQLValue(value any) any {
	if b, ok := value.([]byte); ok {
		return string(b)
	}
	return value
}

const postgresSchemaQuery = `
SELECT table_schema, table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name, ordinal_position`

const mysqlSchemaQuery = `
SELECT table_schema, table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema = DATABASE()
ORDER BY table_schema, table_name, ordinal_position`
