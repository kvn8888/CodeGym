package environmentregistry

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Both stores must satisfy the registry persistence contract.
var (
	_ Store = (*PostgresStore)(nil)
	_ Store = (*InMemoryStore)(nil)
)

// staticRow is a pgx.Row whose Scan copies canned column values into the
// caller's destinations. It lets the record codec be tested without a
// database.
type staticRow struct {
	values []any
	err    error
}

func (r staticRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan: %d destinations for %d columns", len(dest), len(r.values))
	}
	for i := range dest {
		if err := assignColumn(dest[i], r.values[i]); err != nil {
			return fmt.Errorf("scan column %d: %w", i, err)
		}
	}
	return nil
}

func assignColumn(dest, value any) error {
	switch target := dest.(type) {
	case *string:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("want string, got %T", value)
		}
		*target = text
	case *[]byte:
		switch raw := value.(type) {
		case []byte:
			*target = raw
		case string:
			*target = []byte(raw)
		default:
			return fmt.Errorf("want bytes, got %T", value)
		}
	case *time.Time:
		at, ok := value.(time.Time)
		if !ok {
			return fmt.Errorf("want time, got %T", value)
		}
		*target = at
	case **time.Time:
		if value == nil {
			*target = nil
			return nil
		}
		at, ok := value.(time.Time)
		if !ok {
			return fmt.Errorf("want time, got %T", value)
		}
		*target = &at
	default:
		return fmt.Errorf("unsupported destination %T", dest)
	}
	return nil
}

// fullyPopulatedRecord returns a record with every column exercised.
func fullyPopulatedRecord() ArtifactRecord {
	created := time.Date(2026, 4, 2, 9, 30, 0, 0, time.UTC)
	booted := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	return ArtifactRecord{
		ArtifactID:      "spring-boot-di-services",
		ArtifactVersion: "1.0.0",
		Technology:      "spring-boot",
		EnvironmentBase: "eclipse-temurin:21-jdk",
		DependencyFiles: []string{"pom.xml"},
		Lockfiles:       []string{"pom.xml"},
		SetupCommand:    "mvn -q dependency:go-offline",
		BuildCommand:    "mvn -q package -DskipTests",
		TestCommand:     "mvn -q test",
		SnapshotRef:     "snapshot-abc123",
		Provenance: Provenance{
			BuiltBy:       "opencode",
			Owner:         "workspace-1",
			BuilderRunID:  "build_1",
			VerifierRunID: "verify_1",
		},
		Owner:           "workspace-1",
		CreatedAt:       created,
		ValidationState: ValidationPassed,
		LastBootCheckAt: booted,
		Compatibility: CompatibilityRules{
			Technology: "spring-boot",
			Objective:  "practice dependency injection with services",
			ReuseTags:  []string{"demo", "java"},
		},
		Evidence: EvidenceRef{
			BuilderRunID:  "build_1",
			VerifierRunID: "verify_1",
			Verdict:       "passed",
			CheckCount:    4,
		},
	}
}

// TestScanArtifactRecordRoundTrips proves every column decodes back into the
// original record, including the JSONB fields and the nullable boot time.
func TestScanArtifactRecordRoundTrips(t *testing.T) {
	original := fullyPopulatedRecord()
	encoded, err := encodeArtifact(original)
	if err != nil {
		t.Fatalf("encode artifact: %v", err)
	}
	row := staticRow{values: []any{
		original.ArtifactID,
		original.ArtifactVersion,
		original.Technology,
		original.EnvironmentBase,
		[]byte(encoded.dependencyFiles),
		[]byte(encoded.lockfiles),
		original.SetupCommand,
		original.BuildCommand,
		original.TestCommand,
		original.SnapshotRef,
		[]byte(encoded.provenance),
		original.Owner,
		original.CreatedAt,
		string(original.ValidationState),
		original.LastBootCheckAt,
		[]byte(encoded.compatibility),
		[]byte(encoded.evidence),
	}}
	decoded, err := scanArtifactRecord(row)
	if err != nil {
		t.Fatalf("scan artifact: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", decoded, original)
	}
}

// TestScanArtifactRecordNullBootCheck proves a record that was never
// boot-checked decodes with a zero timestamp instead of failing.
func TestScanArtifactRecordNullBootCheck(t *testing.T) {
	row := staticRow{values: []any{
		"artifact", "1", "go", "golang:1.27", []byte("[]"), []byte("[]"),
		"", "go build ./...", "go test ./...", "snapshot-1", []byte("{}"),
		"owner", time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), string(ValidationPassed),
		nil, []byte("{}"), []byte("{}"),
	}}
	decoded, err := scanArtifactRecord(row)
	if err != nil {
		t.Fatalf("scan artifact: %v", err)
	}
	if !decoded.LastBootCheckAt.IsZero() {
		t.Fatalf("expected zero boot-check time, got %s", decoded.LastBootCheckAt)
	}
}

// TestEncodeArtifactNormalizesNilSlices proves nil slices persist as empty
// JSON arrays so nothing stores JSON null.
func TestEncodeArtifactNormalizesNilSlices(t *testing.T) {
	encoded, err := encodeArtifact(ArtifactRecord{})
	if err != nil {
		t.Fatalf("encode artifact: %v", err)
	}
	for name, value := range map[string]string{
		"dependency files": encoded.dependencyFiles,
		"lockfiles":        encoded.lockfiles,
	} {
		if value != "[]" {
			t.Fatalf("%s encoded as %q, want []", name, value)
		}
	}
	if encoded.compatibility == "" {
		t.Fatal("compatibility encoded as empty string")
	}
}

// TestOptionalTimestamp proves the zero time becomes SQL NULL.
func TestOptionalTimestamp(t *testing.T) {
	if optionalTimestamp(time.Time{}) != nil {
		t.Fatal("zero time should map to NULL")
	}
	at := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	got := optionalTimestamp(at)
	if got == nil || !got.Equal(at) {
		t.Fatalf("optionalTimestamp(%s) = %v", at, got)
	}
}

// TestScanArtifactRecordPropagatesNoRows proves the not-found path stays
// detectable: GetArtifact translates exactly this error.
func TestScanArtifactRecordPropagatesNoRows(t *testing.T) {
	_, err := scanArtifactRecord(staticRow{err: pgx.ErrNoRows})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}
