package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type SyncRunStore struct {
	pool *pgxpool.Pool
}

func NewSyncRunStore(pool *pgxpool.Pool) *SyncRunStore {
	return &SyncRunStore{pool: pool}
}

func (s *SyncRunStore) Create(ctx context.Context, run *models.SyncRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sync_runs (id, provider, sync_type, status, records_processed, started_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`,
		run.ID, run.Provider, run.SyncType, run.Status, run.RecordsProcessed,
	)
	return err
}

// Complete marks the sync run as COMPLETED or FAILED and records the final counts.
func (s *SyncRunStore) Complete(ctx context.Context, id uuid.UUID, recordsProcessed int, errMsg *string) error {
	status := "COMPLETED"
	if errMsg != nil {
		status = "FAILED"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE sync_runs SET
			status            = $2,
			records_processed = $3,
			error_message     = $4,
			completed_at      = $5
		WHERE id = $1`,
		id, status, recordsProcessed, errMsg, time.Now().UTC(),
	)
	return err
}

func (s *SyncRunStore) ListByProvider(ctx context.Context, provider string, limit int) ([]models.SyncRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, provider, sync_type, status, records_processed, error_message, started_at, completed_at
		FROM sync_runs WHERE provider = $1
		ORDER BY started_at DESC LIMIT $2`,
		provider, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.SyncRun
	for rows.Next() {
		var r models.SyncRun
		var errMsg *string
		if err := rows.Scan(
			&r.ID, &r.Provider, &r.SyncType, &r.Status,
			&r.RecordsProcessed, &errMsg, &r.StartedAt, &r.CompletedAt,
		); err != nil {
			return nil, err
		}
		r.ErrorMessage = errMsg
		result = append(result, r)
	}
	return result, rows.Err()
}
