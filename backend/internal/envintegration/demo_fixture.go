package envintegration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

// SpringBootPaths names the pinned Spring Boot demonstration exercise files.
// They are exported so the reviewed assessment configuration and the demo
// end-to-end test can reference the same paths the fixture writes. The paths
// are a fixed exercise, not a technology branch in the pipeline: any runtime
// may produce a Spring Boot environment; this is one deterministic stand-in
// used when no agent engine is configured (see FixtureAgentRuntime).
const (
	SpringBootPomPath        = "pom.xml"
	SpringBootApplicationPath = "src/main/java/dev/codegym/demo/Application.java"
	SpringBootControllerPath  = "src/main/java/dev/codegym/demo/HealthController.java"
	SpringBootHiddenTestPath  = ".codegym/hidden/HealthControllerTest.java"
	SpringBootCommandsPath    = ".codegym/commands.json"
)

// springBootPom pins Spring Boot 2.6.2: it is the parent version present in
// the demonstration offline Maven cache, so a cold build resolves with no
// network. The exercise is a GET /health endpoint returning status=ok and
// stack=spring, matching the reviewed assessment below.
const springBootPom = `<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>2.6.2</version>
    <relativePath/>
  </parent>
  <groupId>dev.codegym</groupId>
  <artifactId>codegym-spring-demo</artifactId>
  <version>0.0.1-SNAPSHOT</version>
  <properties><java.version>17</java.version></properties>
  <dependencies>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-web</artifactId>
    </dependency>
    <dependency>
      <groupId>org.junit.jupiter</groupId>
      <artifactId>junit-jupiter</artifactId>
      <version>5.8.2</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-maven-plugin</artifactId>
      </plugin>
    </plugins>
  </build>
</project>
`

// springBootApplication is the Spring Boot entrypoint. It compiles for every
// solution; the learner never edits it.
const springBootApplication = `package dev.codegym.demo;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class Application {
    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }
}
`

// springBootControllerStub is the starting controller a learner completes.
// The reviewed correct and wrong solutions in the assessment configuration
// replace exactly this file.
const springBootControllerStub = `package dev.codegym.demo;

import java.util.Map;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class HealthController {
    @GetMapping("/health")
    public Map<String, String> health() {
        // TODO: GET /health must return JSON with status=ok and stack=spring.
        return Map.of("status", "todo", "stack", "spring");
    }
}
`

// springBootHiddenTest grades the controller with the CODEGYM_TEST protocol.
// It lives under .codegym/hidden/ so it is protected and counted as a hidden
// assessment test; the test command stages it into src/test before running
// Maven. The test prints the protocol verdict and always exits 0: the
// as-built stub must pass its own checks for verification to proceed, the
// verification assessor differentiates correct/wrong solutions from the
// protocol lines, and browser practice evaluates the same protocol lines on
// Submit. A test that failed the build on the stub could never verify.
const springBootHiddenTest = `package dev.codegym.demo;

import java.util.Map;
import org.junit.jupiter.api.Test;

public class HealthControllerTest {
    @Test
    public void healthReturnsOkAndStack() {
        Map<String, String> body = new HealthController().health();
        String status = body.get("status");
        String stack = body.get("stack");
        if ("ok".equals(status) && "spring".equals(stack)) {
            System.out.println("CODEGYM_TEST t1 PASS");
        } else {
            System.out.println("CODEGYM_TEST t1 FAIL code=WRONG_VALUE expected=ok actual=" + status);
        }
    }
}
`

// springBootCommands declares the build steps. Setup is a no-op because the
// offline cache already carries dependencies; build compiles; test stages the
// hidden test and runs Maven so its protocol lines reach the backend.
const springBootCommands = `{
  "setup": "true",
  "build": "mvn -o -q -DskipTests test-compile",
  "test": "mkdir -p src/test/java/dev/codegym/demo && cp .codegym/hidden/HealthControllerTest.java src/test/java/dev/codegym/demo/HealthControllerTest.java && mvn -o -q test",
  "run": "mvn -o -q spring-boot:run",
  "base": "openjdk-17",
  "network": "setup-only"
}
`

// SpringBootFixtureFiles returns the deterministic Spring Boot demonstration
// exercise (with the incomplete controller stub) that FixtureAgentRuntime
// writes into a workspace. Callers must treat it as read-only data.
func SpringBootFixtureFiles() map[string]string {
	return map[string]string{
		SpringBootPomPath:         springBootPom,
		SpringBootApplicationPath: springBootApplication,
		SpringBootControllerPath:  springBootControllerStub,
		SpringBootHiddenTestPath:  springBootHiddenTest,
		SpringBootCommandsPath:    springBootCommands,
	}
}

// FixtureAgentRuntime is a demonstration-only agent runtime that materializes
// a pinned exercise instead of driving a live model. It exists so the whole
// environment pipeline — build handoff, verification, reviewed assessment,
// promotion, and browser practice — can be exercised end to end with no model
// credentials and no network. It is never a substitute for the real agent:
// the built environment still travels the same verifier and registry gates,
// and it must never serve production learners.
type FixtureAgentRuntime struct {
	Files map[string]string
}

// NewSpringBootFixtureRuntime returns a fixture runtime for the pinned Spring
// Boot exercise.
func NewSpringBootFixtureRuntime() *FixtureAgentRuntime {
	return &FixtureAgentRuntime{Files: SpringBootFixtureFiles()}
}

// Name identifies the fixture runtime for provenance.
func (r *FixtureAgentRuntime) Name() string { return "fixture" }

// Run writes the pinned exercise into the task workspace and reports a
// completed run. It ignores the interpolated instructions by design: the
// output is fixed data, not a model result.
func (r *FixtureAgentRuntime) Run(ctx context.Context, task agentruntime.TaskSpec) (agentruntime.RunResult, error) {
	if r == nil || len(r.Files) == 0 {
		return agentruntime.RunResult{}, fmt.Errorf("envintegration: fixture runtime has no files")
	}
	root := strings.TrimSpace(task.WorkingDirectory)
	if root == "" {
		return agentruntime.RunResult{}, fmt.Errorf("envintegration: fixture runtime requires a working directory")
	}
	if err := ctx.Err(); err != nil {
		return agentruntime.RunResult{}, err
	}
	for path, content := range r.Files {
		clean := filepath.Clean(filepath.FromSlash(path))
		if clean == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return agentruntime.RunResult{}, fmt.Errorf("envintegration: fixture file escapes the workspace: %q", path)
		}
		full := filepath.Join(root, clean)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return agentruntime.RunResult{}, fmt.Errorf("envintegration: create fixture directory: %w", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return agentruntime.RunResult{}, fmt.Errorf("envintegration: write fixture file %q: %w", path, err)
		}
	}
	result := agentruntime.RunResult{}
	result.Telemetry.Runtime = "fixture"
	result.Telemetry.Turns = 1
	result.Telemetry.Termination = agentruntime.TerminationCompleted
	return result, nil
}
