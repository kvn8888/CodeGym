package benchmark

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

// Readiness describes whether saved local inputs can be executed and whether
// the preserved generation provenance is complete enough for reporting.
// Invalid configuration is returned as an error; runnable-but-incomplete
// provenance is not an error.
type Readiness struct {
	CaseID                string
	DevelopmentFixture    bool
	Runnable              bool
	ProvenanceComplete    bool
	MissingProvenance     []string
	GovernedContentSHA256 string
	Identity              *ContentIdentity
}

func ValidateSavedCaseReadiness(caseDir, baselineKind string) (Readiness, error) {
	if err := validateBaselineKind(baselineKind); err != nil {
		return Readiness{}, err
	}
	c, err := problemquality.LoadCase(caseDir)
	if err != nil {
		return Readiness{}, err
	}
	config, err := LoadRunConfig(filepath.Join(c.Dir, RunConfigFileName), c)
	if err != nil {
		return Readiness{}, err
	}
	identity, err := computeContentIdentity(c, config)
	if err != nil {
		return Readiness{}, err
	}
	if err := validateGenerationObservationIdentity(config, identity, baselineKind); err != nil {
		return Readiness{}, err
	}
	digest, err := identity.SHA256()
	if err != nil {
		return Readiness{}, fmt.Errorf("compute governed content identity: %w", err)
	}
	missing := missingProvenance(config, baselineKind)
	return Readiness{
		CaseID:                c.CaseID,
		DevelopmentFixture:    config.DevelopmentFixture,
		Runnable:              true,
		ProvenanceComplete:    len(missing) == 0,
		MissingProvenance:     missing,
		GovernedContentSHA256: digest,
		Identity:              identity,
	}, nil
}

func missingProvenance(config *RunConfig, baselineKind string) []string {
	if config.GenerationProvenance == nil {
		return []string{"generation_provenance"}
	}
	provenance := config.GenerationProvenance
	var missing []string
	if strings.TrimSpace(provenance.ModelRevision) == "" {
		missing = append(missing, "generation_provenance.model_revision")
	}
	if strings.TrimSpace(provenance.PromptRevision) == "" {
		missing = append(missing, "generation_provenance.prompt_revision")
	}
	if len(provenance.Config) == 0 {
		missing = append(missing, "generation_provenance.config")
	}
	if provenance.Usage == nil {
		missing = append(missing, "generation_provenance.usage")
	} else {
		if provenance.Usage.InputTokens == nil {
			missing = append(missing, "generation_provenance.usage.input_tokens")
		}
		if provenance.Usage.OutputTokens == nil {
			missing = append(missing, "generation_provenance.usage.output_tokens")
		}
		if provenance.Usage.CostMicros == nil {
			missing = append(missing, "generation_provenance.usage.cost_micros")
		} else {
			if strings.TrimSpace(provenance.Usage.CostStatus) == "" {
				missing = append(missing, "generation_provenance.usage.cost_status")
			}
			if strings.TrimSpace(provenance.Usage.Currency) == "" {
				missing = append(missing, "generation_provenance.usage.currency")
			}
		}
	}
	if strings.TrimSpace(baselineKind) == "independent" && provenance.Observations == nil {
		missing = append(missing, "generation_provenance.observations")
	}
	return missing
}
