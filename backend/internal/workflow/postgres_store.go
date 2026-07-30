package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS workflow_operations (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			kind text NOT NULL,
			status text NOT NULL DEFAULT 'queued',
			last_sequence bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			completed_at timestamptz,
			CONSTRAINT fk_workflow_operations_membership
				FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE,
			CONSTRAINT chk_workflow_kind CHECK (
				kind IN ('mcq_generation', 'memory_reflection', 'mcq_next_round')
			),
			CONSTRAINT chk_workflow_status CHECK (
				status IN ('queued', 'running', 'succeeded', 'failed')
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_operations_scope_updated
			ON workflow_operations (workspace_id, user_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS workflow_progress_events (
			operation_id text NOT NULL REFERENCES workflow_operations (id) ON DELETE CASCADE,
			sequence bigint NOT NULL,
			step_id text NOT NULL,
			label text NOT NULL,
			status text NOT NULL,
			occurred_at timestamptz NOT NULL DEFAULT now(),
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (operation_id, sequence),
			CONSTRAINT chk_workflow_event_status CHECK (
				status IN ('queued', 'running', 'succeeded', 'failed')
			)
		)`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) CreateOperation(ctx context.Context, operation Operation) (Operation, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workflow_operations (
			id, workspace_id, user_id, kind, status, last_sequence,
			created_at, updated_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, operation.ID, operation.WorkspaceID, operation.UserID, operation.Kind,
		operation.Status, operation.LastSequence, operation.CreatedAt.UTC(),
		operation.UpdatedAt.UTC(), nullableTime(operation.CompletedAt))
	if err != nil {
		return Operation{}, err
	}
	return s.GetOperation(ctx, operation.WorkspaceID, operation.UserID, operation.ID)
}

func (s *PostgresStore) GetOperation(
	ctx context.Context,
	workspaceID, userID, operationID string,
) (Operation, error) {
	operation, err := scanOperation(s.pool.QueryRow(ctx, `
		SELECT id, workspace_id, user_id, kind, status, last_sequence,
			created_at, updated_at, completed_at
		FROM workflow_operations
		WHERE id=$1 AND workspace_id=$2 AND user_id=$3
	`, operationID, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	return operation, err
}

func (s *PostgresStore) AppendEvent(
	ctx context.Context,
	workspaceID, userID, operationID string,
	event Event,
	operationStatus Status,
) (Operation, Event, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Operation{}, Event{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	operation, err := scanOperation(tx.QueryRow(ctx, `
		SELECT id, workspace_id, user_id, kind, status, last_sequence,
			created_at, updated_at, completed_at
		FROM workflow_operations
		WHERE id=$1 AND workspace_id=$2 AND user_id=$3
		FOR UPDATE
	`, operationID, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, Event{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, Event{}, err
	}
	if operation.Status == StatusSucceeded || operation.Status == StatusFailed {
		return Operation{}, Event{}, ErrTerminal
	}

	event.OperationID = operation.ID
	event.Sequence = operation.LastSequence + 1
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return Operation{}, Event{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workflow_progress_events (
			operation_id, sequence, step_id, label, status, occurred_at, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, event.OperationID, event.Sequence, event.StepID, event.Label, event.Status,
		event.Timestamp.UTC(), metadata); err != nil {
		return Operation{}, Event{}, err
	}

	var completedAt *time.Time
	if operationStatus == StatusSucceeded || operationStatus == StatusFailed {
		value := event.Timestamp.UTC()
		completedAt = &value
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_operations
		SET status=$1, last_sequence=$2, updated_at=$3, completed_at=$4
		WHERE id=$5
	`, operationStatus, event.Sequence, event.Timestamp.UTC(), nullableTime(completedAt),
		operation.ID); err != nil {
		return Operation{}, Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, Event{}, err
	}
	operation.Status = operationStatus
	operation.LastSequence = event.Sequence
	operation.UpdatedAt = event.Timestamp.UTC()
	operation.CompletedAt = completedAt
	return operation, event, nil
}

func (s *PostgresStore) ListEvents(
	ctx context.Context,
	workspaceID, userID, operationID string,
	afterSequence int64,
	limit int,
) ([]Event, error) {
	if _, err := s.GetOperation(ctx, workspaceID, userID, operationID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.operation_id, e.sequence, e.step_id, e.label, e.status,
			e.occurred_at, e.metadata
		FROM workflow_progress_events e
		JOIN workflow_operations o ON o.id=e.operation_id
		WHERE e.operation_id=$1 AND e.sequence>$2
			AND o.workspace_id=$3 AND o.user_id=$4
		ORDER BY e.sequence ASC
		LIMIT $5
	`, operationID, afterSequence, workspaceID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var event Event
		var metadata []byte
		if err := rows.Scan(
			&event.OperationID, &event.Sequence, &event.StepID, &event.Label,
			&event.Status, &event.Timestamp, &metadata,
		); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, err
			}
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOperation(row rowScanner) (Operation, error) {
	var operation Operation
	err := row.Scan(
		&operation.ID, &operation.WorkspaceID, &operation.UserID, &operation.Kind,
		&operation.Status, &operation.LastSequence, &operation.CreatedAt,
		&operation.UpdatedAt, &operation.CompletedAt,
	)
	return operation, err
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
