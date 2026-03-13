package problemstore

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kevinc/codegym/internal/domain"
	"gopkg.in/yaml.v3"
)

// Store provides read access to problems stored on the filesystem.
type Store struct {
	baseDir  string
	mu       sync.RWMutex
	problems map[string]*domain.Problem // keyed by problem ID
}

func New(baseDir string) *Store {
	return &Store{
		baseDir:  baseDir,
		problems: make(map[string]*domain.Problem),
	}
}

// LoadAll walks the problems directory and indexes all problem.yaml files.
func (s *Store) LoadAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.problems = make(map[string]*domain.Problem)

	err := filepath.WalkDir(s.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "problem.yaml" {
			return nil
		}

		prob, err := s.loadProblem(path)
		if err != nil {
			slog.Warn("skipping problem", "path", path, "error", err)
			return nil
		}

		if _, exists := s.problems[prob.ID]; exists {
			slog.Warn("duplicate problem ID", "id", prob.ID, "path", path)
			return nil
		}

		s.problems[prob.ID] = prob
		slog.Info("loaded problem", "id", prob.ID, "title", prob.Title)
		return nil
	})

	if err != nil {
		return fmt.Errorf("walk problems dir: %w", err)
	}

	slog.Info("problem store loaded", "count", len(s.problems))
	return nil
}

func (s *Store) loadProblem(yamlPath string) (*domain.Problem, error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", yamlPath, err)
	}

	var prob domain.Problem
	if err := yaml.Unmarshal(data, &prob); err != nil {
		return nil, fmt.Errorf("parse %s: %w", yamlPath, err)
	}

	if prob.ID == "" {
		return nil, fmt.Errorf("problem at %s has no id", yamlPath)
	}

	prob.DirPath = filepath.Dir(yamlPath)
	return &prob, nil
}

// Get returns a problem by ID.
func (s *Store) Get(id string) (*domain.Problem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prob, ok := s.problems[id]
	if !ok {
		return nil, fmt.Errorf("problem not found: %s", id)
	}
	return prob, nil
}

// List returns problems matching the given filters.
func (s *Store) List(filters ProblemFilters) []domain.ProblemSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []domain.ProblemSummary
	for _, prob := range s.problems {
		if !filters.Match(prob) {
			continue
		}
		results = append(results, prob.Summary())
	}
	return results
}

// ReadFile reads a file from within a problem's directory.
func (s *Store) ReadFile(problemID, relPath string) ([]byte, error) {
	prob, err := s.Get(problemID)
	if err != nil {
		return nil, err
	}

	// Prevent directory traversal
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") {
		return nil, fmt.Errorf("invalid path: %s", relPath)
	}

	fullPath := filepath.Join(prob.DirPath, clean)
	return os.ReadFile(fullPath)
}

// GetSkeletonFiles returns the skeleton file contents for a problem.
func (s *Store) GetSkeletonFiles(problemID string) ([]domain.SubmissionFile, error) {
	prob, err := s.Get(problemID)
	if err != nil {
		return nil, err
	}

	var files []domain.SubmissionFile
	for _, ref := range prob.Files.Skeleton {
		content, err := os.ReadFile(filepath.Join(prob.DirPath, "skeleton", ref.Path))
		if err != nil {
			// Try without skeleton/ subdirectory (single file problems)
			content, err = os.ReadFile(filepath.Join(prob.DirPath, ref.Path))
			if err != nil {
				return nil, fmt.Errorf("read skeleton file %s: %w", ref.Path, err)
			}
		}
		files = append(files, domain.SubmissionFile{
			Path:    ref.Path,
			Content: string(content),
		})
	}
	return files, nil
}

// ProblemFilters controls which problems are returned by List.
type ProblemFilters struct {
	Language   string
	Category   string
	Difficulty int
	Tag        string
	Framework  string
}

func (f ProblemFilters) Match(p *domain.Problem) bool {
	if f.Language != "" && !strings.EqualFold(p.Language, f.Language) {
		return false
	}
	if f.Category != "" && !strings.EqualFold(p.Category, f.Category) {
		return false
	}
	if f.Framework != "" && !strings.EqualFold(p.Framework, f.Framework) {
		return false
	}
	if f.Difficulty > 0 && p.Difficulty != f.Difficulty {
		return false
	}
	if f.Tag != "" {
		found := false
		for _, t := range p.Tags {
			if strings.EqualFold(t, f.Tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
