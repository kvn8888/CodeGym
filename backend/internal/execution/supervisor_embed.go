package execution

import _ "embed"

// SupervisorScript is uploaded into each sandbox by DaytonaRunner.
//
//go:embed supervisor.py
var SupervisorScript []byte
