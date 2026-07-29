package intake

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS practice_intakes (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			normalized_topic text NOT NULL,
			original_topic text NOT NULL,
			practice_seed jsonb NOT NULL,
			questions jsonb NOT NULL DEFAULT '[]'::jsonb,
			answers jsonb NOT NULL DEFAULT '{}'::jsonb,
			status text NOT NULL,
			suppression_reason text NOT NULL DEFAULT '',
			generation_error text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL,
			completed_at timestamptz,
			FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id) ON DELETE CASCADE,
			CHECK (status IN ('pending','completed','skipped'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_practice_intakes_scope_topic
			ON practice_intakes (workspace_id, user_id, normalized_topic, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_practice_intakes_pending
			ON practice_intakes (workspace_id, user_id, updated_at DESC) WHERE status = 'pending'`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) Create(ctx context.Context, record Record) (Record, error) {
	questions, answers, err := encodeCollections(record)
	if err != nil {
		return Record{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO practice_intakes
		(id,workspace_id,user_id,normalized_topic,original_topic,practice_seed,questions,answers,status,suppression_reason,generation_error,created_at,updated_at,completed_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10,$11,$12,$13,$14)`,
		record.ID, record.WorkspaceID, record.UserID, record.NormalizedTopic, record.OriginalTopic,
		record.PracticeSeed, questions, answers, record.Status, record.SuppressionReason,
		record.GenerationError, record.CreatedAt.UTC(), record.UpdatedAt.UTC(), record.CompletedAt)
	if err != nil {
		return Record{}, err
	}
	return s.Get(ctx, record.WorkspaceID, record.UserID, record.ID)
}

func (s *PostgresStore) Get(ctx context.Context, workspaceID, userID, id string) (Record, error) {
	record, err := scanRecord(s.pool.QueryRow(ctx, selectIntake+` WHERE id=$1 AND workspace_id=$2 AND user_id=$3`, id, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return record, err
}

func (s *PostgresStore) FindLatestByTopic(ctx context.Context, workspaceID, userID, topic string) (Record, error) {
	record, err := scanRecord(s.pool.QueryRow(ctx, selectIntake+` WHERE workspace_id=$1 AND user_id=$2 AND normalized_topic=$3 ORDER BY updated_at DESC LIMIT 1`, workspaceID, userID, topic))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return record, err
}

func (s *PostgresStore) ListPending(ctx context.Context, workspaceID, userID string, limit int) ([]Record, error) {
	rows, err := s.pool.Query(ctx, selectIntake+` WHERE workspace_id=$1 AND user_id=$2 AND status='pending' ORDER BY updated_at DESC LIMIT $3`, workspaceID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *PostgresStore) Update(ctx context.Context, record Record) (Record, error) {
	questions, answers, err := encodeCollections(record)
	if err != nil {
		return Record{}, err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE practice_intakes SET
		questions=$4::jsonb, answers=$5::jsonb, status=$6, suppression_reason=$7,
		generation_error=$8, updated_at=$9, completed_at=$10
		WHERE id=$1 AND workspace_id=$2 AND user_id=$3`,
		record.ID, record.WorkspaceID, record.UserID, questions, answers, record.Status,
		record.SuppressionReason, record.GenerationError, record.UpdatedAt.UTC(), record.CompletedAt)
	if err != nil {
		return Record{}, err
	}
	if tag.RowsAffected() == 0 {
		return Record{}, ErrNotFound
	}
	return s.Get(ctx, record.WorkspaceID, record.UserID, record.ID)
}

const selectIntake = `SELECT id,workspace_id,user_id,normalized_topic,original_topic,
	practice_seed::text,questions::text,answers::text,status,suppression_reason,generation_error,
	created_at,updated_at,completed_at FROM practice_intakes`

type rowScanner interface{ Scan(...any) error }

func scanRecord(row rowScanner) (Record, error) {
	var record Record
	var practiceSeedJSON, questionsJSON, answersJSON []byte
	if err := row.Scan(&record.ID, &record.WorkspaceID, &record.UserID, &record.NormalizedTopic,
		&record.OriginalTopic, &practiceSeedJSON, &questionsJSON, &answersJSON, &record.Status,
		&record.SuppressionReason, &record.GenerationError, &record.CreatedAt, &record.UpdatedAt,
		&record.CompletedAt); err != nil {
		return Record{}, err
	}
	record.PracticeSeed = append(json.RawMessage(nil), practiceSeedJSON...)
	if err := json.Unmarshal(questionsJSON, &record.Questions); err != nil {
		return Record{}, err
	}
	if err := json.Unmarshal(answersJSON, &record.Answers); err != nil {
		return Record{}, err
	}
	return record, nil
}

func encodeCollections(record Record) (string, string, error) {
	questions, err := json.Marshal(record.Questions)
	if err != nil {
		return "", "", err
	}
	answers, err := json.Marshal(record.Answers)
	if err != nil {
		return "", "", err
	}
	return string(questions), string(answers), nil
}
