// Package environment defines the deployment environments that are allowed to
// participate in CodeGym's structural isolation boundaries.
package environment

import (
	"fmt"
	"strings"
)

// Name is a validated CodeGym deployment environment.
type Name string

const (
	Dev Name = "dev"
	Stg Name = "stg"
	Prd Name = "prd"
)

// Parse accepts only the canonical environment names used by CodeGym.
func Parse(raw string) (Name, error) {
	value := Name(strings.ToLower(strings.TrimSpace(raw)))
	if !value.Valid() {
		return "", fmt.Errorf("unrecognised CodeGym environment %q (must be dev, stg, or prd)", raw)
	}
	return value, nil
}

// Valid reports whether the name is one of CodeGym's canonical environments.
func (n Name) Valid() bool {
	switch n {
	case Dev, Stg, Prd:
		return true
	default:
		return false
	}
}
