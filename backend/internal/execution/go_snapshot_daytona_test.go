package execution

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

// TestDaytonaDefaultSnapshotGoProbe records whether Daytona's mutable default
// image already has a usable Go compiler. It is diagnostic; the registered
// snapshot acceptance gate below is the deterministic requirement.
func TestDaytonaDefaultSnapshotGoProbe(t *testing.T) {
	client := daytonaTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sandbox := createSnapshotSandbox(t, ctx, client, "", false, true, "go-default-probe")
	defer deleteSnapshotSandbox(t, sandbox)

	result, err := sandbox.Process.ExecuteCommand(ctx, "command -v go && go version && workdir=$(mktemp -d) && printf 'package main\\nfunc main(){}\\n' > \"$workdir/main.go\" && go build -o \"$workdir/smoke\" \"$workdir/main.go\"")
	if err != nil {
		t.Fatalf("probe default snapshot: %v", err)
	}
	if result.ExitCode != 0 {
		t.Logf("Daytona default snapshot has no usable Go toolchain (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Result))
		return
	}
	t.Logf("Daytona default snapshot currently has usable Go: %s", strings.TrimSpace(result.Result))
}

// TestBuildGoSnapshot is the documented, opt-in snapshot builder. It creates a
// network-enabled sandbox, installs the pinned official Go toolchain, warms the
// persistent run-user GOCACHE with representative HTTP imports, promotes the
// sandbox, then validates the promoted snapshot with all network access blocked.
func TestBuildGoSnapshot(t *testing.T) {
	if os.Getenv("CODEGYM_BUILD_GO_SNAPSHOT") != "1" {
		t.Skip("set CODEGYM_BUILD_GO_SNAPSHOT=1 to build the registered Go snapshot")
	}
	client := daytonaTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if existing, err := client.Snapshot.Get(ctx, GoSnapshotName); err == nil {
		if os.Getenv("CODEGYM_REPLACE_GO_SNAPSHOT") != "1" {
			t.Logf("snapshot %s already exists; validating it (set CODEGYM_REPLACE_GO_SNAPSHOT=1 to rebuild)", GoSnapshotName)
			validateGoSnapshot(t, ctx, client)
			return
		}
		if err := client.Snapshot.Delete(ctx, existing); err != nil {
			t.Fatalf("delete existing snapshot %s: %v", GoSnapshotName, err)
		}
	}

	builder := createSnapshotSandbox(t, ctx, client, "", false, false, "go-snapshot-build")
	defer deleteSnapshotSandbox(t, builder)

	installCommand := fmt.Sprintf(`set -eu
architecture=$(uname -m)
case "$architecture" in
  x86_64) go_arch=amd64 ;;
  aarch64|arm64) go_arch=arm64 ;;
  *) echo "unsupported architecture: $architecture" >&2; exit 2 ;;
esac
if [ "$(id -u)" -eq 0 ]; then privilege=""; elif command -v sudo >/dev/null 2>&1; then privilege=sudo; else echo "root or sudo is required" >&2; exit 2; fi
archive=$(mktemp)
curl -fsSL "https://go.dev/dl/go%s.linux-${go_arch}.tar.gz" -o "$archive"
$privilege rm -rf /usr/local/go
$privilege tar -C /usr/local -xzf "$archive"
$privilege ln -sf /usr/local/go/bin/go /usr/local/bin/go
go version
cache=$(go env GOCACHE)
mkdir -p "$cache"
test -r "$cache" && test -w "$cache"
workdir=$(mktemp -d)
cat > "$workdir/main.go" <<'EOF'
package main

import (
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "os"
  "sync"
)

func main() {
  var mu sync.Mutex
  mux := http.NewServeMux()
  mux.HandleFunc("GET /health", func(response http.ResponseWriter, _ *http.Request) {
    mu.Lock()
    defer mu.Unlock()
    response.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(response).Encode(map[string]bool{"healthy": true})
  })
  response := httptest.NewRecorder()
  mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
  if response.Code != http.StatusOK {
    os.Exit(1)
  }
  _, _ = os.Stdout.Write(response.Body.Bytes())
}
EOF
started_ns=$(date +%%s%%N)
go build -o "$workdir/smoke" "$workdir/main.go"
finished_ns=$(date +%%s%%N)
output=$("$workdir/smoke")
test "$output" = '{"healthy":true}'
printf 'gocache=%%s warm_compile_ms=%%d output=%%s\n' "$cache" "$(((finished_ns-started_ns)/1000000))" "$output"`, GoToolchainVersion)
	result, err := builder.Process.ExecuteCommand(ctx, installCommand, options.WithExecuteTimeout(6*time.Minute))
	if err != nil {
		t.Fatalf("install Go toolchain: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("install Go toolchain exited %d: %s", result.ExitCode, result.Result)
	}
	t.Logf("builder verification: %s", strings.TrimSpace(result.Result))

	if err := builder.ExperimentalCreateSnapshotWithTimeout(ctx, GoSnapshotName, 10*time.Minute); err != nil {
		t.Fatalf("promote snapshot %s: %v", GoSnapshotName, err)
	}
	t.Logf("promoted Daytona snapshot %s", GoSnapshotName)
	validateGoSnapshot(t, ctx, client)
}

// TestGoSnapshot is the mandatory practice-runtime gate: the registered
// snapshot must compile and execute Go while NetworkBlockAll is active.
func TestGoSnapshot(t *testing.T) {
	client := daytonaTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	validateGoSnapshot(t, ctx, client)
}

func validateGoSnapshot(t *testing.T, ctx context.Context, client *daytona.Client) {
	t.Helper()
	sandbox := createSnapshotSandbox(t, ctx, client, GoSnapshotName, true, true, "go-snapshot-validate")
	defer deleteSnapshotSandbox(t, sandbox)
	if err := sandbox.FileSystem.CreateFolder(ctx, "work"); err != nil {
		t.Fatalf("create smoke work directory: %v", err)
	}
	program := []byte(`package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
)

func main() {
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]bool{"healthy": true})
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK {
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(response.Body.Bytes())
}
`)
	if err := sandbox.FileSystem.UploadFile(ctx, program, "work/main.go"); err != nil {
		t.Fatalf("upload smoke program: %v", err)
	}
	result, err := sandbox.Process.ExecuteCommand(ctx,
		`cd ~/work && set -eu
go version
cache=$(go env GOCACHE)
test -r "$cache" && test -w "$cache"
started_ns=$(date +%s%N)
go build -o smoke main.go
finished_ns=$(date +%s%N)
output=$(./smoke)
test "$output" = '{"healthy":true}'
printf 'gocache=%s compile_ms=%d output=%s\n' "$cache" "$(((finished_ns-started_ns)/1000000))" "$output"`,
		options.WithExecuteTimeout(45*time.Second),
	)
	if err != nil {
		t.Fatalf("validate Go snapshot: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Go snapshot validation exited %d: %s", result.ExitCode, result.Result)
	}
	t.Logf("network-blocked snapshot %s compiled and ran a Go HTTP program: %s", GoSnapshotName, strings.TrimSpace(result.Result))
}

func daytonaTestClient(t *testing.T) *daytona.Client {
	t.Helper()
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via doppler run -p codegym -c dev")
	}
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{APIKey: apiKey, APIUrl: os.Getenv("DAYTONA_API_URL")})
	if err != nil {
		t.Fatalf("create Daytona client: %v", err)
	}
	return client
}

func createSnapshotSandbox(t *testing.T, ctx context.Context, client *daytona.Client, snapshot string, networkBlocked, ephemeral bool, track string) *daytona.Sandbox {
	t.Helper()
	sandbox, err := client.Create(ctx, types.SnapshotParams{
		Snapshot: snapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          map[string]string{"codegym": "snapshot", "track": track},
			NetworkBlockAll: networkBlocked,
			Ephemeral:       ephemeral,
		},
	})
	if err != nil {
		t.Fatalf("create Daytona sandbox snapshot=%q: %v", snapshot, err)
	}
	return sandbox
}

func deleteSnapshotSandbox(t *testing.T, sandbox *daytona.Sandbox) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sandbox.Delete(ctx); err != nil {
		t.Errorf("delete Daytona sandbox: %v", err)
	}
}
