package agentruntime

// SpringBootFixture is intentionally excluded from BenchmarkFixtures. It is a
// best-effort extension whose offline Maven dependency cache must be proven
// before it can join the bounded two-stack comparison.
func SpringBootFixture() BenchmarkFixture {
	const pom = `<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>3.3.5</version>
    <relativePath/>
  </parent>
  <groupId>dev.codegym</groupId>
  <artifactId>codegym-spring-fixture</artifactId>
  <version>0.0.1-SNAPSHOT</version>
  <properties><java.version>17</java.version></properties>
  <dependencies>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-web</artifactId>
    </dependency>
  </dependencies>
  <build><plugins><plugin><groupId>org.springframework.boot</groupId><artifactId>spring-boot-maven-plugin</artifactId></plugin></plugins></build>
</project>
`
	const application = `package dev.codegym.fixture;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Map;

@SpringBootApplication
public class Application {
    public static void main(String[] args) {
		SpringApplication application = new SpringApplication(Application.class);
		application.setDefaultProperties(Map.of(
			"server.address", "127.0.0.1",
			"server.port", System.getenv("PORT")
		));
		application.run(args);
    }
}
`
	const controller = `package dev.codegym.fixture;

import java.util.Map;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class HealthController {
    @GetMapping("/health")
    public Map<String, String> health() {
        return Map.of("status", "ok", "stack", "spring");
    }
}
`
	return BenchmarkFixture{
		ID: "spring-boot", Stack: "Spring Boot", EndpointPath: "/health",
		ExpectedPayload: `{"stack":"spring","status":"ok"}`,
		TaskPrompt:      "Build a Spring Boot service in this workspace. GET /health must return JSON with status=ok and stack=spring. Bind to 127.0.0.1:$PORT, use the pinned Maven dependencies, build and test it, and leave no runtime dependency on network egress.",
		BaseFiles:       []FixtureFile{{Path: "pom.xml", Content: pom}},
		GoldenFiles: []FixtureFile{
			{Path: "src/main/java/dev/codegym/fixture/Application.java", Content: application},
			{Path: "src/main/java/dev/codegym/fixture/HealthController.java", Content: controller},
		},
		Broken: []BrokenFixture{
			{Name: "wrong-payload", Files: []FixtureFile{
				{Path: "src/main/java/dev/codegym/fixture/Application.java", Content: application},
				{Path: "src/main/java/dev/codegym/fixture/HealthController.java", Content: replaceFixture(controller, `"status", "ok"`, `"status", "broken"`)},
			}},
			{Name: "wrong-endpoint", Files: []FixtureFile{
				{Path: "src/main/java/dev/codegym/fixture/Application.java", Content: application},
				{Path: "src/main/java/dev/codegym/fixture/HealthController.java", Content: replaceFixture(controller, `@GetMapping("/health")`, `@GetMapping("/ready")`)},
			}},
		},
	}
}
