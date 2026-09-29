package settings

import (
	"errors"
	"time"
)

const (
	// HedgeCountKey controls how many independent Daytona sandbox placement
	// attempts may race for one execution. One preserves the legacy path.
	HedgeCountKey     = "execution.hedge_count"
	DefaultHedgeCount = 1
	MinHedgeCount     = 1
	MaxHedgeCount     = 3
)

type ValueType string

const (
	ValueTypeInt  ValueType = "int"
	ValueTypeBool ValueType = "bool"
)

var (
	ErrSettingNotFound = errors.New("runtime setting not found")
	ErrUnknownKey      = errors.New("unknown runtime setting key")
	ErrInvalidValue    = errors.New("invalid runtime setting value")
)

// Setting is the raw persisted representation. Keeping the raw value lets the
// service detect and safely default corrupt rows instead of failing requests.
type Setting struct {
	WorkspaceID     string
	Key             string
	Type            ValueType
	RawValue        string
	UpdatedByUserID string
	UpdatedAt       time.Time
}

// ResolvedSetting is the typed operator-facing representation returned by the
// authenticated settings endpoint.
type ResolvedSetting struct {
	Key       string     `json:"key"`
	Type      ValueType  `json:"type"`
	Value     any        `json:"value"`
	Default   any        `json:"default"`
	Source    string     `json:"source"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type definition struct {
	key          string
	valueType    ValueType
	defaultValue any
	minimum      int
	maximum      int
}

var definitions = map[string]definition{
	HedgeCountKey: {
		key:          HedgeCountKey,
		valueType:    ValueTypeInt,
		defaultValue: DefaultHedgeCount,
		minimum:      MinHedgeCount,
		maximum:      MaxHedgeCount,
	},
}
