package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
)

const specialNumber = "$codegym_number"

type conformanceRow struct {
	Kind     string   `json:"kind"`
	Epsilon  *float64 `json:"epsilon"`
	Expected any      `json:"expected"`
	Actual   any      `json:"actual"`
}

type conformanceResult struct {
	Index int     `json:"index"`
	Equal bool    `json:"equal"`
	Error *string `json:"error"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: comparator-go TABLE OUTPUT")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var rows []conformanceRow
	if err := decoder.Decode(&rows); err != nil {
		panic(err)
	}
	results := make([]conformanceResult, 0, len(rows))
	for index, row := range rows {
		equal, compareErr := compareValues(row.Kind, reviveSpecial(row.Expected), reviveSpecial(row.Actual), row.Epsilon)
		var errorText *string
		if compareErr != nil {
			message := compareErr.Error()
			errorText = &message
		}
		results = append(results, conformanceResult{Index: index, Equal: equal, Error: errorText})
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[2], encoded, 0o600); err != nil {
		panic(err)
	}
}

func reviveSpecial(value any) any {
	switch typed := value.(type) {
	case []any:
		for index := range typed {
			typed[index] = reviveSpecial(typed[index])
		}
		return typed
	case map[string]any:
		if len(typed) == 1 {
			if marker, ok := typed[specialNumber].(string); ok {
				switch marker {
				case "nan":
					return math.NaN()
				case "+inf":
					return math.Inf(1)
				case "-inf":
					return math.Inf(-1)
				}
			}
		}
		for key, item := range typed {
			typed[key] = reviveSpecial(item)
		}
		return typed
	default:
		return value
	}
}
