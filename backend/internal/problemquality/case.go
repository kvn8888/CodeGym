package problemquality

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"gopkg.in/yaml.v3"
)

const CaseFileName = "case.yaml"

var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

type ErrorKind string

const (
	InvalidCase    ErrorKind = "invalid"
	IncompleteCase ErrorKind = "incomplete"
)

type CaseError struct {
	CaseID  string
	CaseDir string
	Field   string
	Kind    ErrorKind
	Err     error
}

func (e *CaseError) Error() string {
	name := strings.TrimSpace(e.CaseID)
	if name == "" {
		name = filepath.Base(e.CaseDir)
	}
	prefix := "problem-quality case"
	if name != "" {
		prefix = fmt.Sprintf("problem-quality case %q", name)
	}
	if e.Field != "" {
		return fmt.Sprintf("%s: %s: %v", prefix, e.Field, e.Err)
	}
	return fmt.Sprintf("%s: %v", prefix, e.Err)
}

func (e *CaseError) Unwrap() error {
	return e.Err
}

func IsIncomplete(err error) bool {
	var caseErr *CaseError
	return errors.As(err, &caseErr) && caseErr.Kind == IncompleteCase
}

type Case struct {
	CaseID                 string                 `yaml:"case_id"`
	Version                string                 `yaml:"version"`
	Technology             string                 `yaml:"technology"`
	Objective              string                 `yaml:"objective"`
	EnvironmentManifest    string                 `yaml:"environment_manifest"`
	BuildTestRun           BuildTestRun           `yaml:"build_test_run"`
	Spec                   Spec                   `yaml:"spec"`
	CorrectAlternatives    []Solution             `yaml:"correct_alternatives"`
	WrongSolutions         []WrongSolution        `yaml:"wrong_solutions"`
	ComparatorExpectations ComparatorExpectations `yaml:"comparator_expectations"`
	ExpectedOutcomes       *ExpectedOutcomes      `yaml:"expected_outcomes"`
	Review                 Review                 `yaml:"review"`

	Dir                     string                      `yaml:"-"`
	EnvironmentManifestPath string                      `yaml:"-"`
	EnvironmentManifestData *environmentverify.Manifest `yaml:"-"`
	ArtifactID              string                      `yaml:"-"`
	ArtifactVersion         string                      `yaml:"-"`
	SpecPath                string                      `yaml:"-"`
	SpecInline              bool                        `yaml:"-"`
}

type BuildTestRun struct {
	Strategy   string   `yaml:"strategy"`
	Entrypoint string   `yaml:"entrypoint,omitempty"`
	Commands   Commands `yaml:"commands"`
	Limits     *Limits  `yaml:"limits,omitempty"`
}

type Commands struct {
	Setup string `yaml:"setup,omitempty"`
	Build string `yaml:"build,omitempty"`
	Test  string `yaml:"test"`
	Run   string `yaml:"run,omitempty"`
}

type Limits struct {
	TimeoutSeconds *int   `yaml:"timeout_seconds,omitempty"`
	MemoryMB       *int   `yaml:"memory_mb,omitempty"`
	NetworkMode    string `yaml:"network_mode,omitempty"`
}

type Spec struct {
	PathOrInline                 string `yaml:"path_or_inline"`
	AmbiguityResolutionsRequired *bool  `yaml:"ambiguity_resolutions_required,omitempty"`
}

type Solution struct {
	ID         string            `yaml:"id"`
	Files      map[string]string `yaml:"files"`
	ReviewedBy string            `yaml:"reviewed_by"`
	ReviewedAt string            `yaml:"reviewed_at"`

	ResolvedFiles map[string]ResolvedFile `yaml:"-"`
}

type WrongSolution struct {
	ID                         string            `yaml:"id"`
	Files                      map[string]string `yaml:"files"`
	IntendedFailure            IntendedFailure   `yaml:"intended_failure"`
	LearningObjectiveRationale string            `yaml:"learning_objective_rationale"`
	ReviewedBy                 string            `yaml:"reviewed_by"`
	ReviewedAt                 string            `yaml:"reviewed_at"`
	DenominatorDecision        string            `yaml:"denominator_decision,omitempty"`

	ResolvedFiles map[string]ResolvedFile `yaml:"-"`
}

type IntendedFailure struct {
	TestID   string  `yaml:"test_id"`
	Code     string  `yaml:"code"`
	Expected *string `yaml:"expected"`
	Actual   *string `yaml:"actual"`
	Reason   string  `yaml:"reason"`
}

type ComparatorExpectations struct {
	Default              ComparatorDefault `yaml:"default"`
	CaseOverridesAllowed *bool             `yaml:"case_overrides_allowed,omitempty"`
	Notes                string            `yaml:"notes,omitempty"`
}

type ComparatorDefault struct {
	Kind    string   `yaml:"kind"`
	Epsilon *float64 `yaml:"epsilon,omitempty"`
}

type ExpectedOutcomes struct {
	Generation *GenerationExpectations `yaml:"generation,omitempty"`
	Assessment *AssessmentExpectations `yaml:"assessment,omitempty"`
}

type GenerationExpectations struct {
	BlindAgreement          *bool   `yaml:"blind_agreement,omitempty"`
	ReconciliationCount     *int    `yaml:"reconciliation_count,omitempty"`
	ReconciliationDirection *string `yaml:"reconciliation_direction,omitempty"`
	NonConvergence          *bool   `yaml:"non_convergence,omitempty"`
	Status                  string  `yaml:"status,omitempty"`
}

type AssessmentExpectations struct {
	CorrectAlternativesPass *bool `yaml:"correct_alternatives_pass,omitempty"`
	WrongSolutionsRejected  *bool `yaml:"wrong_solutions_rejected,omitempty"`
}

type Review struct {
	ReviewedBy    string `yaml:"reviewed_by"`
	ReviewedAt    string `yaml:"reviewed_at"`
	AmbiguousSpec *bool  `yaml:"ambiguous_spec,omitempty"`
	Notes         string `yaml:"notes,omitempty"`
}

type ResolvedFile struct {
	Target  string
	Source  string
	Path    string
	Content string
}

type CorpusCase struct {
	Dir  string
	Case *Case
	Err  error
}

func LoadCase(caseDir string) (*Case, error) {
	dir, err := filepath.Abs(caseDir)
	if err != nil {
		return nil, &CaseError{CaseDir: caseDir, Kind: InvalidCase, Err: err}
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, &CaseError{CaseDir: dir, Kind: InvalidCase, Err: fmt.Errorf("resolve case directory: %w", err)}
	}
	info, err := os.Stat(resolvedDir)
	if err != nil {
		return nil, &CaseError{CaseDir: resolvedDir, Kind: InvalidCase, Err: fmt.Errorf("read case directory: %w", err)}
	}
	if !info.IsDir() {
		return nil, &CaseError{CaseDir: resolvedDir, Kind: InvalidCase, Err: errors.New("case path is not a directory")}
	}

	casePath := filepath.Join(resolvedDir, CaseFileName)
	data, err := os.ReadFile(casePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &CaseError{CaseDir: resolvedDir, Field: CaseFileName, Kind: IncompleteCase,
				Err: fmt.Errorf("%s is missing", CaseFileName)}
		}
		return nil, &CaseError{CaseDir: resolvedDir, Field: CaseFileName, Kind: InvalidCase,
			Err: fmt.Errorf("read %s: %w", CaseFileName, err)}
	}

	var c Case
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return nil, &CaseError{CaseDir: resolvedDir, Field: CaseFileName, Kind: InvalidCase,
			Err: fmt.Errorf("parse %s: %w", CaseFileName, err)}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, &CaseError{CaseID: c.CaseID, CaseDir: resolvedDir, Field: CaseFileName, Kind: InvalidCase,
			Err: fmt.Errorf("%s must contain exactly one document", CaseFileName)}
	}
	c.Dir = resolvedDir

	if c.Spec.AmbiguityResolutionsRequired == nil {
		value := true
		c.Spec.AmbiguityResolutionsRequired = &value
	}
	if c.ComparatorExpectations.CaseOverridesAllowed == nil {
		value := true
		c.ComparatorExpectations.CaseOverridesAllowed = &value
	}
	for index := range c.WrongSolutions {
		if strings.TrimSpace(c.WrongSolutions[index].DenominatorDecision) == "" {
			c.WrongSolutions[index].DenominatorDecision = "meaningful"
		}
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	if err := c.resolveReferences(); err != nil {
		return nil, err
	}
	return &c, nil
}

func LoadCorpus(casesRoot string) ([]CorpusCase, error) {
	entries, err := os.ReadDir(casesRoot)
	if err != nil {
		return nil, fmt.Errorf("read problem-quality corpus %q: %w", casesRoot, err)
	}
	results := make([]CorpusCase, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(casesRoot, entry.Name())
		loaded, loadErr := LoadCase(dir)
		results = append(results, CorpusCase{Dir: dir, Case: loaded, Err: loadErr})
	}
	return results, nil
}

func (c *Case) validate() error {
	if strings.TrimSpace(c.CaseID) == "" {
		return c.invalid("case_id", "is required")
	}
	if !versionPattern.MatchString(strings.TrimSpace(c.Version)) {
		return c.invalid("version", "must be a semantic version like 1.0.0, got %q", c.Version)
	}
	if strings.TrimSpace(c.Technology) == "" {
		return c.invalid("technology", "is required")
	}
	if strings.TrimSpace(c.Objective) == "" {
		return c.invalid("objective", "is required")
	}
	if strings.TrimSpace(c.EnvironmentManifest) == "" {
		return c.invalid("environment_manifest", "is required")
	}
	if err := c.validateBuildTestRun(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Spec.PathOrInline) == "" {
		return c.invalid("spec.path_or_inline", "is required")
	}
	if err := c.validateComparator(); err != nil {
		return err
	}
	if err := c.validateExpectedOutcomes(); err != nil {
		return err
	}
	if err := c.validateReview(); err != nil {
		return err
	}

	if len(c.CorrectAlternatives) == 0 {
		return c.invalid("correct_alternatives", "must contain at least one solution")
	}
	if len(c.WrongSolutions) == 0 {
		return c.invalid("wrong_solutions", "must contain at least one solution")
	}

	ids := make(map[string]string, len(c.CorrectAlternatives)+len(c.WrongSolutions))
	for index := range c.CorrectAlternatives {
		if err := c.validateSolution(index, &c.CorrectAlternatives[index], ids); err != nil {
			return err
		}
	}
	for index := range c.WrongSolutions {
		if err := c.validateWrongSolution(index, &c.WrongSolutions[index], ids); err != nil {
			return err
		}
	}
	return nil
}

func (c *Case) validateBuildTestRun() error {
	switch strings.TrimSpace(c.BuildTestRun.Strategy) {
	case "unit", "http", "output", "custom":
	default:
		return c.invalid("build_test_run.strategy", "must be unit, http, output, or custom, got %q", c.BuildTestRun.Strategy)
	}
	if strings.TrimSpace(c.BuildTestRun.Commands.Test) == "" {
		return c.invalid("build_test_run.commands.test", "is required")
	}
	if c.BuildTestRun.Limits != nil {
		limits := c.BuildTestRun.Limits
		if limits.TimeoutSeconds != nil && *limits.TimeoutSeconds < 1 {
			return c.invalid("build_test_run.limits.timeout_seconds", "must be at least 1")
		}
		if limits.MemoryMB != nil && *limits.MemoryMB < 1 {
			return c.invalid("build_test_run.limits.memory_mb", "must be at least 1")
		}
	}
	return nil
}

func (c *Case) validateComparator() error {
	switch strings.TrimSpace(c.ComparatorExpectations.Default.Kind) {
	case "exact", "set", "multiset", "sorted", "float", "checker":
	default:
		return c.invalid("comparator_expectations.default.kind",
			"must be exact, set, multiset, sorted, float, or checker, got %q",
			c.ComparatorExpectations.Default.Kind)
	}
	if epsilon := c.ComparatorExpectations.Default.Epsilon; epsilon != nil && *epsilon <= 0 {
		return c.invalid("comparator_expectations.default.epsilon", "must be greater than 0")
	}
	return nil
}

func (c *Case) validateExpectedOutcomes() error {
	if c.ExpectedOutcomes == nil {
		return c.invalid("expected_outcomes", "is required")
	}
	generation := c.ExpectedOutcomes.Generation
	if generation == nil {
		return nil
	}
	if direction := generation.ReconciliationDirection; direction != nil {
		switch strings.TrimSpace(*direction) {
		case "reference", "tests", "both":
		default:
			return c.invalid("expected_outcomes.generation.reconciliation_direction",
				"must be reference, tests, or both, got %q", *direction)
		}
	}
	if generation.ReconciliationCount != nil && *generation.ReconciliationCount < 0 {
		return c.invalid("expected_outcomes.generation.reconciliation_count", "must not be negative")
	}
	if status := strings.TrimSpace(generation.Status); status != "" {
		switch status {
		case "measured", "not_applicable":
		default:
			return c.invalid("expected_outcomes.generation.status",
				"must be measured or not_applicable, got %q", generation.Status)
		}
	}
	return nil
}

func (c *Case) validateReview() error {
	if strings.TrimSpace(c.Review.ReviewedBy) == "" {
		return c.invalid("review.reviewed_by", "is required")
	}
	if err := validateDateTime(c.Review.ReviewedAt); err != nil {
		return c.invalid("review.reviewed_at", "%v", err)
	}
	return nil
}

func (c *Case) validateSolution(index int, solution *Solution, ids map[string]string) error {
	path := fmt.Sprintf("correct_alternatives[%d]", index)
	if err := c.validateSolutionIdentity(path, solution.ID, solution.ReviewedBy, solution.ReviewedAt, ids); err != nil {
		return err
	}
	return c.validateFiles(path, solution.Files)
}

func (c *Case) validateWrongSolution(index int, solution *WrongSolution, ids map[string]string) error {
	path := fmt.Sprintf("wrong_solutions[%d]", index)
	if err := c.validateSolutionIdentity(path, solution.ID, solution.ReviewedBy, solution.ReviewedAt, ids); err != nil {
		return err
	}
	if err := c.validateFiles(path, solution.Files); err != nil {
		return err
	}

	failure := solution.IntendedFailure
	if strings.TrimSpace(failure.TestID) == "" {
		return c.invalid(path+".intended_failure.test_id", "is required")
	}
	if strings.TrimSpace(failure.Code) == "" {
		return c.invalid(path+".intended_failure.code", "is required")
	}
	if failure.Expected == nil {
		return c.invalid(path+".intended_failure.expected", "is required")
	}
	if failure.Actual == nil {
		return c.invalid(path+".intended_failure.actual", "is required")
	}
	if strings.TrimSpace(failure.Reason) == "" {
		return c.invalid(path+".intended_failure.reason", "is required")
	}
	if strings.TrimSpace(solution.LearningObjectiveRationale) == "" {
		return c.invalid(path+".learning_objective_rationale", "is required")
	}
	switch strings.TrimSpace(solution.DenominatorDecision) {
	case "meaningful", "equivalent", "invalid":
	default:
		return c.invalid(path+".denominator_decision",
			"must be meaningful, equivalent, or invalid, got %q", solution.DenominatorDecision)
	}
	return nil
}

func (c *Case) validateSolutionIdentity(path, id, reviewedBy, reviewedAt string, ids map[string]string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return c.invalid(path+".id", "is required")
	}
	if previous, exists := ids[id]; exists {
		return c.invalid(path+".id", "duplicates solution id %q already used by %s", id, previous)
	}
	ids[id] = path
	if strings.TrimSpace(reviewedBy) == "" {
		return c.invalid(path+".reviewed_by", "is required")
	}
	if err := validateDateTime(reviewedAt); err != nil {
		return c.invalid(path+".reviewed_at", "%v", err)
	}
	return nil
}

func (c *Case) validateFiles(path string, files map[string]string) error {
	if len(files) == 0 {
		return c.invalid(path+".files", "must include at least one file mapping")
	}
	keys := make([]string, 0, len(files))
	for target := range files {
		keys = append(keys, target)
	}
	sort.Strings(keys)
	for _, target := range keys {
		field := fmt.Sprintf("%s.files[%q]", path, target)
		if err := validateWorkspacePath(target); err != nil {
			return c.invalid(field, "%v", err)
		}
		if strings.TrimSpace(files[target]) == "" {
			return c.invalid(field, "source path is required")
		}
	}
	return nil
}

func (c *Case) resolveReferences() error {
	manifestPath, err := c.resolveReference(c.EnvironmentManifest, "environment_manifest")
	if err != nil {
		return err
	}
	c.EnvironmentManifestPath = manifestPath
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return c.invalid("environment_manifest", "cannot read environment manifest %q: %v", c.EnvironmentManifest, err)
	}
	manifest, err := environmentverify.ValidateBytes(manifestData)
	if err != nil {
		return c.invalid("environment_manifest", "environment manifest %q is invalid: %v", c.EnvironmentManifest, err)
	}
	c.EnvironmentManifestData = &manifest
	c.ArtifactID = manifest.Artifact.ID
	c.ArtifactVersion = manifest.Artifact.Version

	if isInlineSpec(c.Spec.PathOrInline) {
		c.SpecInline = true
	} else {
		specPath, err := c.resolveReference(c.Spec.PathOrInline, "spec.path_or_inline")
		if err != nil {
			return err
		}
		c.SpecPath = specPath
	}

	for index := range c.CorrectAlternatives {
		files, err := c.resolveFiles(c.CorrectAlternatives[index].Files,
			fmt.Sprintf("correct_alternatives[%d].files", index))
		if err != nil {
			return err
		}
		c.CorrectAlternatives[index].ResolvedFiles = files
	}
	for index := range c.WrongSolutions {
		files, err := c.resolveFiles(c.WrongSolutions[index].Files,
			fmt.Sprintf("wrong_solutions[%d].files", index))
		if err != nil {
			return err
		}
		c.WrongSolutions[index].ResolvedFiles = files
	}
	return nil
}

func (c *Case) resolveFiles(files map[string]string, field string) (map[string]ResolvedFile, error) {
	targets := make([]string, 0, len(files))
	for target := range files {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	resolved := make(map[string]ResolvedFile, len(files))
	for _, target := range targets {
		source := files[target]
		sourceField := fmt.Sprintf("%s[%q]", field, target)
		path, err := c.resolveReference(source, sourceField)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, c.invalid(sourceField, "cannot read source file %q: %v", source, err)
		}
		resolved[target] = ResolvedFile{Target: target, Source: source, Path: path, Content: string(data)}
	}
	return resolved, nil
}

func (c *Case) resolveReference(raw, field string) (string, error) {
	reference := strings.TrimSpace(raw)
	if reference == "" {
		return "", c.invalid(field, "is required")
	}
	slashed := filepath.FromSlash(reference)
	if filepath.IsAbs(slashed) {
		return "", c.invalid(field, "must be relative to the case directory, got %q", raw)
	}
	cleaned := filepath.Clean(slashed)
	if escapesRoot(".", cleaned) {
		return "", c.invalid(field, "escapes the case directory, got %q", raw)
	}
	path := filepath.Join(c.Dir, cleaned)
	if !containsPath(c.Dir, path) {
		return "", c.invalid(field, "escapes the case directory, got %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", c.incomplete(field, "referenced file %q does not exist", raw)
		}
		return "", c.invalid(field, "cannot read referenced file %q: %v", raw, err)
	}
	if !info.Mode().IsRegular() {
		return "", c.invalid(field, "referenced path %q is not a regular file", raw)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", c.invalid(field, "cannot resolve referenced file %q: %v", raw, err)
	}
	if !containsPath(c.Dir, canonical) {
		return "", c.invalid(field, "referenced file %q resolves outside the case directory", raw)
	}
	return canonical, nil
}

func validateWorkspacePath(raw string) error {
	path := strings.TrimSpace(raw)
	if path == "" {
		return errors.New("target path is required")
	}
	slashed := filepath.FromSlash(path)
	if filepath.IsAbs(slashed) {
		return errors.New("target path must be relative")
	}
	cleaned := filepath.Clean(slashed)
	if cleaned == "." || escapesRoot(".", cleaned) {
		return errors.New("target path escapes the workspace")
	}
	return nil
}

func validateDateTime(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("is required")
	}
	if _, err := time.Parse(time.RFC3339, trimmed); err != nil {
		return fmt.Errorf("must be an RFC3339 date-time, got %q", value)
	}
	return nil
}

func isInlineSpec(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.Contains(trimmed, "\n")
}

func escapesRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel)
}

func containsPath(root, path string) bool {
	return !escapesRoot(root, path)
}

func (c *Case) invalid(field, format string, args ...any) *CaseError {
	return &CaseError{
		CaseID:  strings.TrimSpace(c.CaseID),
		CaseDir: c.Dir,
		Field:   field,
		Kind:    InvalidCase,
		Err:     fmt.Errorf(format, args...),
	}
}

func (c *Case) incomplete(field, format string, args ...any) *CaseError {
	return &CaseError{
		CaseID:  strings.TrimSpace(c.CaseID),
		CaseDir: c.Dir,
		Field:   field,
		Kind:    IncompleteCase,
		Err:     fmt.Errorf(format, args...),
	}
}
