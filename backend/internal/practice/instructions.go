package practice

// SampleGreetInstructions is the learner-safe instruction text for the
// development sample exercise. It describes only what the learner must
// implement and how to run checks. It deliberately omits hidden test
// names, hidden assertions, expected answers, reference solutions, and
// the correct/wrong analysis that the greet fixture README documents for
// backend reviewers. Never serve the fixture README to learners.
const SampleGreetInstructions = `Practice: greet a name

Implement ` + "`greet(name)`" + ` in ` + "`solution.py`" + `.

Your function receives one name value and returns a greeting string:

- Ordinary names are greeted directly: a name like Ada produces
  "Hello, Ada!".
- Surrounding whitespace is ignored before greeting.
- Blank input (empty or whitespace-only) returns exactly
  "Hello, stranger!".

` + "`solution.py`" + ` is the only file you can edit.

Run executes the backend test command as a smoke check: this exercise
declares no run command, so no program output is shown. Submit runs the
build command and then the test command. Backend feedback reports only a
category (passed, build-error, test-failure, timeout, or platform-fault)
with a short summary — it never shows assessment details.`
