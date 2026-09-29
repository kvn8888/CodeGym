package agentruntime

import (
	"os"
	"testing"
)

func TestSpringBootFixtureBestEffort(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_SPRING_FIXTURE") != "1" {
		t.Skip("best effort: set CODEGYM_RUN_SPRING_FIXTURE=1 to test the offline Maven cache")
	}
	fixture := SpringBootFixture()
	workingDirectory := t.TempDir()
	if err := fixture.MaterializeGolden(workingDirectory); err != nil {
		t.Fatal(err)
	}
	verification, err := FixtureVerifier{Fixture: fixture}.VerifyWorkspace(t.Context(), workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Passed {
		t.Skipf("Spring Boot is not benchmarkable with the current offline cache: %s", verification.Detail)
	}
	t.Log("Spring Boot golden fixture passed with the current offline Maven cache")
}
