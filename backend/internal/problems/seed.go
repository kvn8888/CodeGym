package problems

import (
	"encoding/json"

	"github.com/kvn8888/codegym/backend/internal/harness"
)

const twoSumSkeleton = `def two_sum(nums: list[int], target: int) -> list[int]:
    """Return the indices of two values whose sum equals target."""
    # TODO: implement this function.
    return []
`

const twoSumReferenceSolution = `def two_sum(nums: list[int], target: int) -> list[int]:
    seen = {}
    for index, value in enumerate(nums):
        complement = target - value
        if complement in seen:
            return [seen[complement], index]
        seen[value] = index
    return []
`

func twoSumDefinition() Definition {
	rendered, err := harness.Render(twoSumHarnessSpec())
	if err != nil {
		panic("render two-sum harness: " + err.Error())
	}
	return Definition{
		Problem: Problem{
			Summary: Summary{
				ID:               "two-sum",
				Title:            "Two Sum",
				Category:         "algorithms",
				Language:         "python",
				Difficulty:       1,
				Tags:             []string{"arrays", "hash-map"},
				EstimatedMinutes: 20,
				Type:             "coding",
			},
			Version:     "1.0.0",
			Subcategory: "arrays-and-hash-maps",
			Description: `Given an array of integers, return the indices of the two numbers whose values add up to a target.

Exactly one valid answer exists. You may not use the same array element twice, and you may return the two indices in either order.

### Example

**Input:** nums = [2, 7, 11, 15], target = 9

**Output:** [0, 1]

### Constraints

- 2 <= len(nums) <= 10,000
- -10^9 <= nums[i], target <= 10^9
- Exactly one valid pair exists.`,
			Runtime: Runtime{
				Image:          "python312",
				TimeoutSeconds: 30,
				MemoryMB:       256,
				NetworkMode:    "block-all",
			},
			Files: FileManifest{Skeleton: []FileRef{
				{Path: "solution.py", Entry: true},
			}},
			TestConfig: TestConfig{Strategy: "unit"},
			Hints: []Hint{
				{Cost: 0, Text: "As you scan the array, ask whether you have already seen the value needed to reach the target."},
				{Cost: 1, Text: "Store each visited value and its index in a dictionary so complement lookup is constant time."},
			},
		},
		SkeletonFiles: []File{
			{Path: "solution.py", Content: twoSumSkeleton},
		},
		HiddenTestFiles: []File{
			{Path: rendered.Path, Content: rendered.Content},
		},
		ReferenceSolution: twoSumReferenceSolution,
		Entrypoint:        "test_solution.py",
		Visibility:        VisibilityGlobal,
	}
}

func twoSumHarnessSpec() harness.Spec {
	return harness.Spec{
		Language:   harness.LanguagePython,
		Module:     "solution",
		EntryPoint: "two_sum",
		ParamNames: []string{"nums", "target"},
		Cases: []harness.Case{
			{
				Name:       "finds a pair in a sorted-looking list",
				Args:       []json.RawMessage{json.RawMessage(`[2,7,11,15]`), json.RawMessage(`9`)},
				Expected:   json.RawMessage(`[0,1]`),
				Comparator: harness.ComparatorUnorderedList,
			},
			{
				Name:       "finds a pair without relying on order",
				Args:       []json.RawMessage{json.RawMessage(`[3,2,4]`), json.RawMessage(`6`)},
				Expected:   json.RawMessage(`[1,2]`),
				Comparator: harness.ComparatorUnorderedList,
			},
			{
				Name:       "handles duplicate values",
				Args:       []json.RawMessage{json.RawMessage(`[3,3]`), json.RawMessage(`6`)},
				Expected:   json.RawMessage(`[0,1]`),
				Comparator: harness.ComparatorUnorderedList,
			},
			{
				Name:       "handles negative values",
				Args:       []json.RawMessage{json.RawMessage(`[-3,4,3,90]`), json.RawMessage(`0`)},
				Expected:   json.RawMessage(`[0,2]`),
				Comparator: harness.ComparatorUnorderedList,
			},
		},
	}
}
