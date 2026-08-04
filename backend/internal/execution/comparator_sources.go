package execution

import _ "embed"

// PythonComparatorSource is uploaded as a server-owned hidden support file for
// Python problem harnesses. The same bytes are used by conformance tests.
//
//go:embed testdata/comparator_python.py
var PythonComparatorSource string
