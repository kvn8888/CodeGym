# Spring Boot test prerequisites

The Spring Boot corpus case declares Maven commands and uses the real Spring
Boot 3.4.2 dependencies. Setup, build, and assessment are separate stages; the
assessment phase only runs the declared test command.

## Prerequisites

- Java 21 (verified with 21.0.2)
- Maven 3.9.x (verified with 3.9.6)
- Spring Boot 3.4.2 from `artifact/pom.xml`
- A populated Maven repository cache or access to Maven Central for the first
  dependency download

The artifact contains source inputs only. Generated `target/` output is not an
input and is ignored by git. Validate from a clean artifact directory:

```bash
rm -rf artifact/target
```

from `benchmarks/problem-quality/cases/spring-boot-greeting-endpoint/`.

## Commands

From `benchmarks/problem-quality/cases/spring-boot-greeting-endpoint/artifact`:

```bash
mvn -q dependency:go-offline
mvn -q test-compile
mvn -q test
```

The case and artifact manifests declare the same commands, so the harness does
not need any Java-specific branch. If Maven Central is unreachable but the
dependency cache is populated, the benchmark command can be run with
`MAVEN_ARGS=-o`; the declared commands stay unchanged. If the cache and network
cannot satisfy `pom.xml`, Spring Boot execution is blocked. Structural checks
and the other language cases still run. Do not report a blocked run as passed.

## Test harness behavior

`artifact/src/test/java/com/codegym/exercise/GreetingEndpointTest.java`:

- starts the Spring Boot application on port `0` (an OS-assigned dynamic port);
- binds the server to `127.0.0.1`;
- bounds startup to 30 seconds and each HTTP request to 3 seconds;
- checks status, JSON content type, response shape, missing/blank names,
  unknown query parameters, and no trimming;
- emits `CODEGYM_TEST` protocol lines for each protected assertion;
- closes the Spring application context in `@AfterAll`.

The protected application class lists `pom.xml`, `GreetingApplication.java`,
and `GreetingEndpointTest.java`. Only
`GreetingController.java` is learner-editable. `maven-surefire-plugin` keeps
printed test output visible (`redirectTestOutputToFile=false`) so the
`CODEGYM_TEST` lines reach the harness.

## Run the benchmark

From `backend/`:

```bash
PATH="$PWD/../.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1 \
  go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case ../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint \
  -development-fixture \
  -output /tmp/spring-boot-greeting-endpoint-report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 300s
```

Run all four milestone cases with explicit repeatable `-case` flags:

```bash
PATH="$PWD/../.venv/bin:$PATH" \
npm_config_offline=true npm_config_audit=false npm_config_fund=false \
PYTHONDONTWRITEBYTECODE=1 \
  go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case ../benchmarks/problem-quality/cases/python-sum-even-numbers \
  -case ../benchmarks/problem-quality/cases/go-count-even-numbers \
  -case ../benchmarks/problem-quality/cases/express-greeting-endpoint \
  -case ../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint \
  -development-fixture \
  -output /tmp/all-four-report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 300s
```

The checked-in case excludes `example-sum`, the intentional survivor fixture.
The local repository-level integration evidence is under
`benchmark-results/integration/`.
