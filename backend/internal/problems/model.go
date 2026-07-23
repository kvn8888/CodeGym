package problems

// Summary is the compact problem-library representation consumed by the
// frontend list page.
type Summary struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Category         string   `json:"category"`
	Language         string   `json:"language"`
	Framework        string   `json:"framework,omitempty"`
	Difficulty       int      `json:"difficulty"`
	Tags             []string `json:"tags"`
	EstimatedMinutes int      `json:"estimated_minutes"`
	Type             string   `json:"type"`
}

// Problem is the public problem specification. It is safe to serialize in an
// HTTP response: executable tests and the reference solution live only on
// Definition below and are excluded from JSON.
type Problem struct {
	Summary
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Subcategory string       `json:"subcategory,omitempty"`
	Runtime     Runtime      `json:"runtime"`
	Files       FileManifest `json:"files"`
	TestConfig  TestConfig   `json:"test_config"`
	Hints       []Hint       `json:"hints,omitempty"`
}

type Runtime struct {
	Image          string `json:"image"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MemoryMB       int    `json:"memory_mb"`
	NetworkMode    string `json:"network_mode"`
}

type FileManifest struct {
	Skeleton []FileRef `json:"skeleton"`
}

type FileRef struct {
	Path     string `json:"path"`
	Entry    bool   `json:"entry,omitempty"`
	Readonly bool   `json:"readonly,omitempty"`
}

type TestConfig struct {
	Strategy string `json:"strategy"`
}

type Hint struct {
	Cost int    `json:"cost"`
	Text string `json:"text"`
}

type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Skeleton struct {
	Files []File `json:"files"`
}

// Definition is the server-side record used to assemble an execution. The
// json:"-" tags are a second line of defense against accidentally exposing
// hidden tests, the entrypoint, or the reference solution from an endpoint.
type Definition struct {
	Problem
	SkeletonFiles     []File `json:"-"`
	HiddenTestFiles   []File `json:"-"`
	ReferenceSolution string `json:"-"`
	Entrypoint        string `json:"-"`
}
