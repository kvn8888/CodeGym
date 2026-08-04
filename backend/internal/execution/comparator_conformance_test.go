package execution

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type comparatorConformanceRow struct {
	WantEqual bool   `json:"want_equal"`
	Note      string `json:"note"`
}

type comparatorConformanceResult struct {
	Index int     `json:"index"`
	Equal bool    `json:"equal"`
	Error *string `json:"error"`
}

func TestComparatorConformance(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for comparator conformance")
	}
	tablePath := filepath.Join("testdata", "comparators.json")
	rawTable, err := os.ReadFile(tablePath)
	if err != nil {
		t.Fatalf("read comparator table: %v", err)
	}
	var rows []comparatorConformanceRow
	if err := json.Unmarshal(rawTable, &rows); err != nil {
		t.Fatalf("decode comparator table: %v", err)
	}

	resultsByLanguage := make(map[string][]comparatorConformanceResult)
	t.Run("python", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "python-results.json")
		command := exec.Command(python, filepath.Join("testdata", "run_python_comparators.py"), tablePath, outputPath)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("run Python comparator conformance: %v\n%s", err, output)
		}
		resultsByLanguage["python"] = assertComparatorResults(t, rows, outputPath)
	})

	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is required for comparator conformance")
	}
	t.Run("go", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "go-results.json")
		command := exec.Command(goBinary, "run",
			filepath.Join("testdata", "comparator_go.go"),
			filepath.Join("testdata", "run_go_comparators.go"),
			tablePath, outputPath,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("run Go comparator conformance: %v\n%s", err, output)
		}
		resultsByLanguage["go"] = assertComparatorResults(t, rows, outputPath)
	})

	pythonResults := resultsByLanguage["python"]
	goResults := resultsByLanguage["go"]
	for index := range rows {
		if pythonResults[index].Equal != goResults[index].Equal {
			t.Errorf("row %d (%s): Python equal=%t, Go equal=%t", index, rows[index].Note, pythonResults[index].Equal, goResults[index].Equal)
		}
	}
}

func assertComparatorResults(t *testing.T, rows []comparatorConformanceRow, path string) []comparatorConformanceResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read comparator results: %v", err)
	}
	var results []comparatorConformanceResult
	if err := json.Unmarshal(raw, &results); err != nil {
		t.Fatalf("decode comparator results: %v", err)
	}
	if len(results) != len(rows) {
		t.Fatalf("result count = %d, want %d", len(results), len(rows))
	}
	for index, result := range results {
		if result.Index != index {
			t.Errorf("row %d (%s): result index = %d", index, rows[index].Note, result.Index)
		}
		if result.Error != nil {
			t.Errorf("row %d (%s): unexpected error: %s", index, rows[index].Note, *result.Error)
		}
		if result.Equal != rows[index].WantEqual {
			t.Errorf("row %d (%s): equal = %t, want %t", index, rows[index].Note, result.Equal, rows[index].WantEqual)
		}
	}
	return results
}
