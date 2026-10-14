package envintegration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// DemoEvidenceSchemaVersion versions the demonstration evidence bundle.
const DemoEvidenceSchemaVersion = "environment.demo-evidence.v1"

// DemoEvidence is one recorded demonstration: the exact application commit
// it ran on, the requested technology, the builder output, the verifier
// verdict with its evidence, and the promotion outcome. It is the acceptance
// artifact for the "both demonstrations complete" criterion: manifests,
// checks run, failure evidence, and telemetry in one place.
type DemoEvidence struct {
	SchemaVersion     string                               `json:"schemaVersion"`
	ApplicationCommit string                               `json:"applicationCommit"`
	Technology        string                               `json:"technology"`
	Objective         string                               `json:"objective"`
	Manifest          environmentverify.Manifest           `json:"manifest"`
	Builder           environmentverify.BuilderResult      `json:"builder"`
	Verification      environmentverify.VerifierResult     `json:"verification"`
	Promotion         environmentregistry.PromotionReceipt `json:"promotion"`
	RecordedAt        time.Time                            `json:"recordedAt"`
}

// AssembleEvidence bundles one completed demonstration. The promotion
// receipt may record refusal; the bundle captures failures as faithfully as
// passes, since failure evidence is a required recording.
func AssembleEvidence(applicationCommit, technology, objective string, build environmentregistry.BuildOutput, verification environmentverify.VerifierResult, receipt environmentregistry.PromotionReceipt) (*DemoEvidence, error) {
	if strings.TrimSpace(applicationCommit) == "" {
		return nil, fmt.Errorf("envintegration: application commit is required")
	}
	if strings.TrimSpace(technology) == "" {
		return nil, fmt.Errorf("envintegration: technology is required")
	}
	return &DemoEvidence{
		SchemaVersion:     DemoEvidenceSchemaVersion,
		ApplicationCommit: applicationCommit,
		Technology:        technology,
		Objective:         objective,
		Manifest:          build.Manifest,
		Builder:           build.Builder,
		Verification:      verification,
		Promotion:         receipt,
		RecordedAt:        time.Now().UTC(),
	}, nil
}

// WriteEvidenceBundle serializes one demonstration bundle to dir under a
// deterministic filename. It creates nothing else and modifies nothing.
func WriteEvidenceBundle(dir, technology string, bundle *DemoEvidence) (string, error) {
	if bundle == nil {
		return "", fmt.Errorf("envintegration: evidence bundle is required")
	}
	payload, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", fmt.Errorf("envintegration: encode evidence bundle: %w", err)
	}
	name := "demo-evidence-" + strings.ToLower(strings.ReplaceAll(strings.TrimSpace(technology), " ", "-")) + ".json"
	if strings.TrimSpace(technology) == "" {
		name = "demo-evidence.json"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("envintegration: write evidence bundle: %w", err)
	}
	return path, nil
}
