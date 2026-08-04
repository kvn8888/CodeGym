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
