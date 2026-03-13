package domain

import "time"

type ProblemType string

const (
	ProblemTypeFunction      ProblemType = "function"
	ProblemTypeAPIServer     ProblemType = "api-server"
	ProblemTypeLibraryUsage  ProblemType = "library-usage"
	ProblemTypeSystem        ProblemType = "system"
	ProblemTypeDesignPattern ProblemType = "design-pattern"
	ProblemTypeOutputMatch   ProblemType = "output-match"
)

type TestStrategy string

const (
	TestStrategyUnit   TestStrategy = "unit"
	TestStrategyHTTP   TestStrategy = "http"
	TestStrategyOutput TestStrategy = "output"
	TestStrategyCustom TestStrategy = "custom"
)

type Problem struct {
	Version          string       `yaml:"version" json:"version"`
	ID               string       `yaml:"id" json:"id"`
	Title            string       `yaml:"title" json:"title"`
	Description      string       `yaml:"description" json:"description"`
	Category         string       `yaml:"category" json:"category"`
	Subcategory      string       `yaml:"subcategory,omitempty" json:"subcategory,omitempty"`
	Language         string       `yaml:"language" json:"language"`
	Framework        string       `yaml:"framework,omitempty" json:"framework,omitempty"`
	Difficulty       int          `yaml:"difficulty" json:"difficulty"`
	Tags             []string     `yaml:"tags" json:"tags"`
	EstimatedMinutes int          `yaml:"estimated_minutes" json:"estimated_minutes"`
	Type             ProblemType  `yaml:"type" json:"type"`
	Runtime          RuntimeConfig `yaml:"runtime" json:"runtime"`
	Files            FileMapping  `yaml:"files" json:"files"`
	TestConfig       TestConfig   `yaml:"test_config" json:"test_config"`
	Hints            []Hint       `yaml:"hints,omitempty" json:"hints,omitempty"`
	Generation       *GenerationMeta `yaml:"generation,omitempty" json:"generation,omitempty"`

	// Set at load time, not from YAML
	DirPath string `yaml:"-" json:"-"`
}

type RuntimeConfig struct {
	Image          string       `yaml:"image" json:"image"`
	BuildCmd       string       `yaml:"build_cmd,omitempty" json:"build_cmd,omitempty"`
	TimeoutSeconds int          `yaml:"timeout_seconds" json:"timeout_seconds"`
	MemoryMB       int          `yaml:"memory_mb" json:"memory_mb"`
	CPUShares      int64        `yaml:"cpu_shares,omitempty" json:"cpu_shares,omitempty"`
	NetworkMode    string       `yaml:"network_mode" json:"network_mode"`
	Ports          []PortConfig `yaml:"ports,omitempty" json:"ports,omitempty"`
}

type PortConfig struct {
	Container int    `yaml:"container" json:"container"`
	Purpose   string `yaml:"purpose,omitempty" json:"purpose,omitempty"`
}

type FileMapping struct {
	Skeleton []FileRef `yaml:"skeleton" json:"skeleton"`
	Solution []FileRef `yaml:"solution" json:"-"` // never expose solution to clients
	Tests    []FileRef `yaml:"tests" json:"-"`    // tests hidden from clients
	Support  []FileRef `yaml:"support,omitempty" json:"-"`
}

type FileRef struct {
	Path     string `yaml:"path" json:"path"`
	Entry    bool   `yaml:"entry,omitempty" json:"entry,omitempty"`
	Readonly bool   `yaml:"readonly,omitempty" json:"readonly,omitempty"`
	Runner   string `yaml:"runner,omitempty" json:"runner,omitempty"`
	Mount    string `yaml:"mount,omitempty" json:"-"`
}

type TestConfig struct {
	Strategy       TestStrategy  `yaml:"strategy" json:"strategy"`
	Setup          []SetupStep   `yaml:"setup,omitempty" json:"-"`
	Run            string        `yaml:"run" json:"-"`
	Teardown       []string      `yaml:"teardown,omitempty" json:"-"`
	ExpectedStdout string        `yaml:"expected_stdout,omitempty" json:"-"`
	Compare        string        `yaml:"compare,omitempty" json:"-"`
}

type SetupStep struct {
	Cmd                string `yaml:"cmd" json:"cmd"`
	WaitFor            string `yaml:"wait_for,omitempty" json:"wait_for,omitempty"`
	WaitTimeoutSeconds int    `yaml:"wait_timeout_seconds,omitempty" json:"wait_timeout_seconds,omitempty"`
}

type Hint struct {
	Cost int    `yaml:"cost" json:"cost"`
	Text string `yaml:"text" json:"text"`
}

type GenerationMeta struct {
	GeneratedBy     string    `yaml:"generated_by" json:"generated_by"`
	GeneratedAt     time.Time `yaml:"generated_at" json:"generated_at"`
	Prompt          string    `yaml:"prompt" json:"prompt"`
	Validated       bool      `yaml:"validated" json:"validated"`
	ValidationRunID string    `yaml:"validation_run_id,omitempty" json:"validation_run_id,omitempty"`
}

// ProblemSummary is a lightweight view for list endpoints.
type ProblemSummary struct {
	ID               string      `json:"id"`
	Title            string      `json:"title"`
	Category         string      `json:"category"`
	Language         string      `json:"language"`
	Framework        string      `json:"framework,omitempty"`
	Difficulty       int         `json:"difficulty"`
	Tags             []string    `json:"tags"`
	EstimatedMinutes int         `json:"estimated_minutes"`
	Type             ProblemType `json:"type"`
}

func (p *Problem) Summary() ProblemSummary {
	return ProblemSummary{
		ID:               p.ID,
		Title:            p.Title,
		Category:         p.Category,
		Language:         p.Language,
		Framework:        p.Framework,
		Difficulty:       p.Difficulty,
		Tags:             p.Tags,
		EstimatedMinutes: p.EstimatedMinutes,
		Type:             p.Type,
	}
}
