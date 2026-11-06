package agentruntime

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkFixtures(t *testing.T) {
	fixtures := BenchmarkFixtures()
	if got := strings.Join(fixtureIDs(fixtures), ","); got != "express,go-net-http" {
		t.Fatalf("fixture ids = %q", got)
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID+"/golden", func(t *testing.T) {
			workingDirectory := t.TempDir()
			if err := fixture.MaterializeGolden(workingDirectory); err != nil {
				t.Fatal(err)
			}
			verification, err := Evaluate(t.Context(), FixtureVerifier{Fixture: fixture}, fixtureTask(workingDirectory), validFixtureRun())
			if err != nil {
				t.Fatal(err)
			}
			if !verification.Passed {
				t.Fatalf("golden verification = %#v", verification)
			}
		})
		if len(fixture.Broken) < 2 {
			t.Fatalf("fixture %s has %d broken variants", fixture.ID, len(fixture.Broken))
		}
		for _, broken := range fixture.Broken {
			broken := broken
			t.Run(fixture.ID+"/rejects-"+broken.Name, func(t *testing.T) {
				workingDirectory := t.TempDir()
				if err := fixture.MaterializeBroken(workingDirectory, broken.Name); err != nil {
					t.Fatal(err)
				}
				verification, err := Evaluate(t.Context(), FixtureVerifier{Fixture: fixture}, fixtureTask(workingDirectory), validFixtureRun())
				if err != nil {
					t.Fatal(err)
				}
				if verification.Passed {
					t.Fatalf("known-broken variant %q passed", broken.Name)
				}
			})
		}
	}
}

func fixtureTask(workingDirectory string) TaskSpec {
	return TaskSpec{
		Goal: "fixture verification", WorkingDirectory: workingDirectory,
		AllowedTools: []Tool{ToolReadFile}, TurnCeiling: 1, Deadline: time.Now().Add(30 * time.Second),
	}
}

func validFixtureRun() RunResult {
	return RunResult{Manifest: ManifestClaim{Status: ManifestPresent, Manifest: &ResultManifest{Version: ManifestVersion, Completed: true}}}
}
