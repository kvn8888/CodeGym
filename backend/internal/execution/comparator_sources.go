package execution

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	goHTTPHarnessHashPlaceholder = "__CODEGYM_HTTP_HARNESS_SHA256__"
	// GoHTTPHarnessSnapshotDir is the immutable snapshot-owned harness location.
	GoHTTPHarnessSnapshotDir = "/opt/codegym/http-harness"
)

// PythonComparatorSource is uploaded as a server-owned hidden support file for
// Python problem harnesses. The same bytes are used by conformance tests.
//
//go:embed testdata/comparator_python.py
var PythonComparatorSource string

// GoComparatorSource is compiled as a server-owned file in every Go harness.
//
//go:embed testdata/comparator_go.go
var GoComparatorSource string

// GoCompileRunnerSource compiles the learner solution plus hidden Go files and
// writes a failed verdict on compiler errors before the harness can start.
//
//go:embed testdata/go_compile_and_run.py
var GoCompileRunnerSource string

// GoHTTPHarnessSource starts a compiled learner server, owns its process group,
// and evaluates hidden HTTP cases with the same Go comparator as unit problems.
//
//go:embed testdata/http_harness.go
var GoHTTPHarnessSource string

// GoHTTPHarnessSourceHash versions the precompiled harness by all source bytes
// that determine verdict behavior. The NUL separator prevents concatenation
// ambiguity between the harness and comparator files.
var GoHTTPHarnessSourceHash = hashGoHTTPHarnessSource()

// GoHTTPCompileRunnerSource compiles the learner server, verifies the
// snapshot-owned harness hash, and either uses the matching precompiled binary
// or safely recompiles the server-owned sources.
//
//go:embed testdata/go_http_compile_and_run.py
var goHTTPCompileRunnerTemplate string

var GoHTTPCompileRunnerSource = renderGoHTTPCompileRunnerSource(GoHTTPHarnessSourceHash)

func hashGoHTTPHarnessSource() string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(GoHTTPHarnessSource))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(GoComparatorSource))
	return hex.EncodeToString(digest.Sum(nil))
}

func renderGoHTTPCompileRunnerSource(hash string) string {
	if strings.Count(goHTTPCompileRunnerTemplate, goHTTPHarnessHashPlaceholder) != 1 {
		panic(fmt.Sprintf("Go HTTP compile runner must contain exactly one %s placeholder", goHTTPHarnessHashPlaceholder))
	}
	return strings.Replace(goHTTPCompileRunnerTemplate, goHTTPHarnessHashPlaceholder, hash, 1)
}
