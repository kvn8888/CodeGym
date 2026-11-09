package environmentregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists promoted artifact records so the registry survives
// backend restarts, following the execution/usage store pattern. Records are
// keyed by artifact id and version, exactly like InMemoryStore, so the two
// implementations are interchangeable behind Store.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a durable registry store on an existing pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema creates the registry table and lookup index. It is
// idempotent and additive so it can run on every boot alongside the other
// store schemas.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS environment_artifacts (
			artifact_id text NOT NULL,
			artifact_version text NOT NULL,
			technology text NOT NULL,
			environment_base text NOT NULL DEFAULT '',
			dependency_files jsonb NOT NULL DEFAULT '[]'::jsonb,
			lockfiles jsonb NOT NULL DEFAULT '[]'::jsonb,
			setup_command text NOT NULL DEFAULT '',
			build_command text NOT NULL DEFAULT '',
			test_command text NOT NULL DEFAULT '',
			snapshot_ref text NOT NULL DEFAULT '',
			provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
			owner text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL,
			validation_state text NOT NULL,
			last_boot_check_at timestamptz,
			compatibility jsonb NOT NULL DEFAULT '{}'::jsonb,
			evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (artifact_id, artifact_version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_environment_artifacts_technology_created_at
			ON environment_artifacts (technology, created_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

const selectArtifactColumns = `SELECT
	artifact_id,
	artifact_version,
	technology,
	environment_base,
	dependency_files,
	lockfiles,
	setup_command,
	build_command,
	test_command,
	snapshot_ref,
	provenance,
	owner,
	created_at,
	validation_state,
	last_boot_check_at,
	compatibility,
	evidence`

// SaveArtifact inserts or replaces the record for one artifact version.
// Replacement on same id/version re-promotion matches InMemoryStore and the
// standing compatibility decision recorded in doc.go.
func (s *PostgresStore) SaveArtifact(ctx context.Context, record ArtifactRecord) error {
	encoded, err := encodeArtifact(record)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO environment_artifacts (
			artifact_id,
			artifact_version,
			technology,
			environment_base,
			dependency_files,
			lockfiles,
			setup_command,
			build_command,
			test_command,
			snapshot_ref,
			provenance,
			owner,
			created_at,
			validation_state,
			last_boot_check_at,
			compatibility,
			evidence
		) VALUES (
			$1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $9, $10,
			$11::jsonb, $12, $13, $14, $15, $16::jsonb, $17::jsonb
		)
		ON CONFLICT (artifact_id, artifact_version) DO UPDATE SET
			technology = EXCLUDED.technology,
			environment_base = EXCLUDED.environment_base,
			dependency_files = EXCLUDED.dependency_files,
			lockfiles = EXCLUDED.lockfiles,
			setup_command = EXCLUDED.setup_command,
			build_command = EXCLUDED.build_command,
			test_command = EXCLUDED.test_command,
			snapshot_ref = EXCLUDED.snapshot_ref,
			provenance = EXCLUDED.provenance,
			owner = EXCLUDED.owner,
			created_at = EXCLUDED.created_at,
			validation_state = EXCLUDED.validation_state,
			last_boot_check_at = EXCLUDED.last_boot_check_at,
			compatibility = EXCLUDED.compatibility,
			evidence = EXCLUDED.evidence
	`, record.ArtifactID, record.ArtifactVersion, record.Technology, record.EnvironmentBase,
		encoded.dependencyFiles, encoded.lockfiles, record.SetupCommand, record.BuildCommand, record.TestCommand,
		record.SnapshotRef, encoded.provenance, record.Owner, record.CreatedAt.UTC(), string(record.ValidationState),
		optionalTimestamp(record.LastBootCheckAt), encoded.compatibility, encoded.evidence)
	return err
}

// ListByTechnology returns promoted records for one technology, newest
// first. Callers apply tag and version matching.
func (s *PostgresStore) ListByTechnology(ctx context.Context, technology string) ([]ArtifactRecord, error) {
	rows, err := s.pool.Query(ctx, selectArtifactColumns+`
		FROM environment_artifacts
		WHERE technology = $1
		ORDER BY created_at DESC
	`, technology)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []ArtifactRecord{}
	for rows.Next() {
		record, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// GetArtifact returns one promoted record or ErrArtifactNotFound.
func (s *PostgresStore) GetArtifact(ctx context.Context, artifactID, version string) (ArtifactRecord, error) {
	row := s.pool.QueryRow(ctx, selectArtifactColumns+`
		FROM environment_artifacts
		WHERE artifact_id = $1 AND artifact_version = $2
	`, artifactID, version)
	record, err := scanArtifactRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArtifactRecord{}, ErrArtifactNotFound
	}
	return record, err
}

// UpdateValidation advances one record's validation state and boot-check
// timestamp, or returns ErrArtifactNotFound.
func (s *PostgresStore) UpdateValidation(ctx context.Context, artifactID, version string, state ValidationState, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE environment_artifacts
		SET validation_state = $3, last_boot_check_at = $4
		WHERE artifact_id = $1 AND artifact_version = $2
	`, artifactID, version, string(state), optionalTimestamp(at))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrArtifactNotFound
	}
	return nil
}

// encodedArtifact holds the JSONB columns of one record as text, ready for
// jsonb binding.
type encodedArtifact struct {
	dependencyFiles string
	lockfiles       string
	provenance      string
	compatibility   string
	evidence        string
}

// encodeArtifact marshals every JSON field. Nil slices become empty arrays
// rather than JSON null so the database always holds arrays.
func encodeArtifact(record ArtifactRecord) (encodedArtifact, error) {
	dependencyFiles, err := encodeStringSlice(record.DependencyFiles)
	if err != nil {
		return encodedArtifact{}, fmt.Errorf("encode dependency files: %w", err)
	}
	lockfiles, err := encodeStringSlice(record.Lockfiles)
	if err != nil {
		return encodedArtifact{}, fmt.Errorf("encode lockfiles: %w", err)
	}
	provenance, err := encodeJSONValue(record.Provenance)
	if err != nil {
		return encodedArtifact{}, fmt.Errorf("encode provenance: %w", err)
	}
	compatibility, err := encodeCompatibility(record.Compatibility)
	if err != nil {
		return encodedArtifact{}, fmt.Errorf("encode compatibility: %w", err)
	}
	evidence, err := encodeJSONValue(record.Evidence)
	if err != nil {
		return encodedArtifact{}, fmt.Errorf("encode evidence: %w", err)
	}
	return encodedArtifact{
		dependencyFiles: dependencyFiles,
		lockfiles:       lockfiles,
		provenance:      provenance,
		compatibility:   compatibility,
		evidence:        evidence,
	}, nil
}

func encodeStringSlice(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	return encodeJSONValue(values)
}

// encodeCompatibility marshals compatibility rules with reuse tags as an
// empty array rather than null, matching the other slice columns.
func encodeCompatibility(rules CompatibilityRules) (string, error) {
	reuseTags := rules.ReuseTags
	if reuseTags == nil {
		reuseTags = []string{}
	}
	return encodeJSONValue(CompatibilityRules{
		Technology: rules.Technology,
		Objective:  rules.Objective,
		ReuseTags:  reuseTags,
	})
}

func encodeJSONValue(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// optionalTimestamp maps the zero time to SQL NULL so a record that has
// never been boot-checked stores no timestamp.
func optionalTimestamp(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	utc := at.UTC()
	return &utc
}

// scanArtifactRecord decodes one row in selectArtifactColumns order. It
// accepts pgx.Row so rows in a List and a single QueryRow share it.
func scanArtifactRecord(row pgx.Row) (ArtifactRecord, error) {
	var (
		record                     ArtifactRecord
		dependencyFiles, lockfiles []byte
		provenance, compatibility  []byte
		evidence                   []byte
		validationState            string
		lastBootCheckAt            *time.Time
	)
	if err := row.Scan(
		&record.ArtifactID,
		&record.ArtifactVersion,
		&record.Technology,
		&record.EnvironmentBase,
		&dependencyFiles,
		&lockfiles,
		&record.SetupCommand,
		&record.BuildCommand,
		&record.TestCommand,
		&record.SnapshotRef,
		&provenance,
		&record.Owner,
		&record.CreatedAt,
		&validationState,
		&lastBootCheckAt,
		&compatibility,
		&evidence,
	); err != nil {
		return ArtifactRecord{}, err
	}
	if err := decodeJSONValue(dependencyFiles, &record.DependencyFiles); err != nil {
		return ArtifactRecord{}, fmt.Errorf("decode dependency files: %w", err)
	}
	if err := decodeJSONValue(lockfiles, &record.Lockfiles); err != nil {
		return ArtifactRecord{}, fmt.Errorf("decode lockfiles: %w", err)
	}
	if err := decodeJSONValue(provenance, &record.Provenance); err != nil {
		return ArtifactRecord{}, fmt.Errorf("decode provenance: %w", err)
	}
	if err := decodeJSONValue(compatibility, &record.Compatibility); err != nil {
		return ArtifactRecord{}, fmt.Errorf("decode compatibility: %w", err)
	}
	if err := decodeJSONValue(evidence, &record.Evidence); err != nil {
		return ArtifactRecord{}, fmt.Errorf("decode evidence: %w", err)
	}
	record.ValidationState = ValidationState(validationState)
	if lastBootCheckAt != nil {
		record.LastBootCheckAt = *lastBootCheckAt
	}
	return record, nil
}

// decodeJSONValue unmarshals a JSONB column, treating an empty column as the
// zero value so legacy rows without a payload still scan.
func decodeJSONValue(raw []byte, target any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}
