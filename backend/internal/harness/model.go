package harness

import "encoding/json"

type Language string

const LanguagePython Language = "python"

type Comparator string

const (
	ComparatorEqual         Comparator = "equal"
	ComparatorUnorderedList Comparator = "unordered_list"
	ComparatorSetEqual      Comparator = "set_equal"
	ComparatorFloatApprox   Comparator = "float_approx"
	ComparatorAnyOf         Comparator = "any_of"
)

type Case struct {
	Name       string
	Args       []json.RawMessage
	Expected   json.RawMessage
	Comparator Comparator
}

type Spec struct {
	Language   Language
	Module     string
	EntryPoint string
	ParamNames []string
	Cases      []Case
}

type Rendered struct {
	Path    string
	Content string
}

func SupportedLanguages() []Language {
	return []Language{LanguagePython}
}

func Comparators() []Comparator {
	return []Comparator{
		ComparatorEqual,
		ComparatorUnorderedList,
		ComparatorSetEqual,
		ComparatorFloatApprox,
		ComparatorAnyOf,
	}
}
