package storage

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.SyncRunRepository = (*SyncRunStore)(nil)

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
// Status must be one of models.SyncRunCompleted or models.SyncRunFailed.
func (s *SyncRunStore) Complete(ctx context.Context, id uuid.UUID, status string, recordsProcessed int, errMsg *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sync_runs SET
			status            = $2,
			records_processed = $3,
			error_message     = $4,
			completed_at      = NOW()
		WHERE id = $1`,
		id, status, recordsProcessed, errMsg,
	)
	return err
}

func (s *SyncRunStore) ListByProvider(ctx context.Context, provider string, limit int) ([]entities.SyncRun, error) {
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

	var result []entities.SyncRun
	for rows.Next() {
		var r entities.SyncRun
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
