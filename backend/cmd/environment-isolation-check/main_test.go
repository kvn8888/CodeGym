package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environment"
)

func TestCheckIsolationReportsOverlapsWithoutSecretMaterial(t *testing.T) {
	values := map[environment.Name]map[string]string{
		environment.Dev: {
			"NEON_CONNECTION_STRING":     "dev-production-database-secret-material",
			"DAYTONA_API_KEY":            "daytona-dev-secret-material",
			"CODEGYM_RELAY_TOKEN_SECRET": "shared-relay-secret-material",
		},
		environment.Stg: {
			"NEON_CONNECTION_STRING":     "staging-database-secret-material",
			"DAYTONA_API_KEY":            "shared-daytona-secret-material",
			"CODEGYM_RELAY_TOKEN_SECRET": "shared-relay-secret-material",
		},
		environment.Prd: {
			"NEON_CONNECTION_STRING":     "dev-production-database-secret-material",
			"DAYTONA_API_KEY":            "shared-daytona-secret-material",
			"CODEGYM_RELAY_TOKEN_SECRET": "shared-relay-secret-material",
		},
	}
	var output bytes.Buffer
	unsafe, err := checkIsolation(context.Background(), &output, func(_ context.Context, name environment.Name) (map[string]string, error) {
		return values[name], nil
	})
	if err != nil {
		t.Fatalf("checkIsolation: %v", err)
	}
	if !unsafe {
		t.Fatal("shared secrets must make the check unsafe")
	}
	report := output.String()
	for _, secrets := range values {
		for _, value := range secrets {
			if strings.Contains(report, value) {
				t.Fatalf("report exposed secret material %q: %s", value, report)
			}
		}
	}
	for _, fragment := range []string{
		"NEON_CONNECTION_STRING", "dev=prd",
		"DAYTONA_API_KEY", "stg=prd",
		"CODEGYM_RELAY_TOKEN_SECRET", "dev=stg=prd",
	} {
		if !strings.Contains(report, fragment) {
			t.Fatalf("report missing %q: %s", fragment, report)
		}
	}
	wantFingerprint := displayFingerprint(sha256.Sum256([]byte("dev-production-database-secret-material")))
	if !strings.Contains(report, wantFingerprint) {
		t.Fatalf("report missing expected fingerprint %s: %s", wantFingerprint, report)
	}
}

func TestCheckIsolationPassesDistinctValues(t *testing.T) {
	var output bytes.Buffer
	unsafe, err := checkIsolation(context.Background(), &output, func(_ context.Context, name environment.Name) (map[string]string, error) {
		return map[string]string{
			"NEON_CONNECTION_STRING":     string(name) + "-database",
			"DAYTONA_API_KEY":            string(name) + "-daytona",
			"CODEGYM_RELAY_TOKEN_SECRET": string(name) + "-relay",
		}, nil
	})
	if err != nil {
		t.Fatalf("checkIsolation: %v", err)
	}
	if unsafe {
		t.Fatalf("distinct secrets reported unsafe: %s", output.String())
	}
	if count := strings.Count(output.String(), "none"); count != len(checkedSecrets) {
		t.Fatalf("distinct report = %s", output.String())
	}
}

func TestCheckIsolationFailsClosedOnMissingSecret(t *testing.T) {
	var output bytes.Buffer
	unsafe, err := checkIsolation(context.Background(), &output, func(_ context.Context, name environment.Name) (map[string]string, error) {
		return map[string]string{
			"NEON_CONNECTION_STRING":     string(name) + "-database",
			"DAYTONA_API_KEY":            string(name) + "-daytona",
			"CODEGYM_RELAY_TOKEN_SECRET": map[environment.Name]string{environment.Dev: "", environment.Stg: "stg-relay", environment.Prd: "prd-relay"}[name],
		}, nil
	})
	if err != nil {
		t.Fatalf("checkIsolation: %v", err)
	}
	if !unsafe || !strings.Contains(output.String(), "MISSING") || !strings.Contains(output.String(), "missing:dev") {
		t.Fatalf("missing secret did not fail closed: %s", output.String())
	}
}
