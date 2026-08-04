package execution

import _ "embed"

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

// GoHTTPCompileRunnerSource compiles the learner server and HTTP harness before
// delegating lifecycle and case execution to the harness binary.
//
//go:embed testdata/go_http_compile_and_run.py
var GoHTTPCompileRunnerSource string
