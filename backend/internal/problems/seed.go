package problems

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

const twoSumHiddenTests = `import importlib
import json
import time

PREFIX = "CODEGYM_RESULT "


def emit(tests, compile_error=None):
    print(PREFIX + json.dumps(
        {"tests": tests, "compile_error": compile_error},
        separators=(",", ":"),
    ))


try:
    solution = importlib.import_module("solution")
except Exception as exc:
    emit([], f"{type(exc).__name__}: {exc}")
    raise SystemExit(0)


cases = [
    ("finds a pair in a sorted-looking list", [2, 7, 11, 15], 9, [0, 1]),
    ("finds a pair without relying on order", [3, 2, 4], 6, [1, 2]),
    ("handles duplicate values", [3, 3], 6, [0, 1]),
    ("handles negative values", [-3, 4, 3, 90], 0, [0, 2]),
]

results = []
for name, nums, target, expected in cases:
    started = time.perf_counter()
    error = None
    status = "pass"
    try:
        actual = solution.two_sum(nums, target)
        if not isinstance(actual, list) or sorted(actual) != sorted(expected):
            status = "fail"
            error = f"expected {expected}, got {actual}"
    except Exception as exc:
        status = "fail"
        error = f"{type(exc).__name__}: {exc}"
    duration_ms = max(0, int((time.perf_counter() - started) * 1000))
    results.append({
        "name": name,
        "status": status,
        "duration_ms": duration_ms,
        "error": error,
    })

emit(results)
`

func twoSumDefinition() Definition {
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
			{Path: "test_solution.py", Content: twoSumHiddenTests},
		},
		ReferenceSolution: twoSumReferenceSolution,
		Entrypoint:        "test_solution.py",
	}
}
