// Package repository provides model-specific persistence operations.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"campfire/internal/model"

	"github.com/mattn/go-sqlite3"
)

// Repository errors distinguish missing records and uniqueness conflicts.
var (
	ErrNotFound = errors.New("spark not found")
	ErrConflict = errors.New("a Spark with that name or a local Spark already exists")
)

// SparkRepository persists Spark records.
type SparkRepository struct{ db *sql.DB }

// NewSparkRepository creates a repository using the shared database pool.
func NewSparkRepository(db *sql.DB) *SparkRepository {
	return &SparkRepository{db: db}
}

// Create inserts a Spark, enforcing unique names and a single local node.
func (r *SparkRepository) Create(ctx context.Context, spark model.Spark) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO sparks(id, name, kind, created_at) VALUES (?, ?, ?, ?)",
		spark.ID, spark.Name, spark.Kind, spark.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		var sqliteError sqlite3.Error
		if errors.As(err, &sqliteError) && (sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique || sqliteError.ExtendedCode == sqlite3.ErrConstraintPrimaryKey) {
			return ErrConflict
		}

		return fmt.Errorf("insert Spark: %w", err)
	}

	return nil
}

// List returns Sparks in creation order.
func (r *SparkRepository) List(ctx context.Context) ([]model.Spark, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, kind, created_at FROM sparks ORDER BY created_at, id")
	if err != nil {
		return nil, fmt.Errorf("list Sparks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	sparks := make([]model.Spark, 0)

	for rows.Next() {
		spark, err := scanSpark(rows)
		if err != nil {
			return nil, err
		}

		sparks = append(sparks, spark)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Sparks: %w", err)
	}

	return sparks, nil
}

// Get looks up a Spark by its opaque identifier.
func (r *SparkRepository) Get(ctx context.Context, id string) (model.Spark, error) {
	return scanSpark(r.db.QueryRowContext(ctx, "SELECT id, name, kind, created_at FROM sparks WHERE id = ?", id))
}

func scanSpark(row interface{ Scan(...any) error }) (model.Spark, error) {
	var spark model.Spark

	var createdAt string
	if err := row.Scan(&spark.ID, &spark.Name, &spark.Kind, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Spark{}, ErrNotFound
		}

		return model.Spark{}, fmt.Errorf("read Spark: %w", err)
	}

	timestamp, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.Spark{}, fmt.Errorf("read Spark timestamp: %w", err)
	}

	spark.CreatedAt = timestamp

	return spark, nil
}
