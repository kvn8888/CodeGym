// Command base-snapshot builds the prebaked base snapshot the production build
// corridor starts from.
//
// It installs the toolchain every generated environment needs — the JDK,
// Maven, Node, Go, Python, the pinned opencode binary, and the opencode plugin
// runtime the custom progress tool imports — verifies each one inside the
// sandbox, and then promotes the sandbox to a named snapshot. Build sandboxes
// are leased from that snapshot, so a build only installs project
// dependencies; it never reinstalls a toolchain.
//
// This is a one-time setup step. Run it with Doppler supplying the Daytona
// credentials, then put the printed snapshot name into CODEGYM_BASE_SNAPSHOT:
//
//	doppler run -p codegym -c dev -- go run ./base-snapshot -name codegym-base
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

// opencodeVersion must match agentruntime.OpenCodePinnedVersion: the sandbox
// runtime refuses to run an agent whose version differs from the pin.
const opencodeVersion = "1.18.11"

// configDir is the opencode config directory the sandbox runtime writes into.
// The plugin runtime is installed here so the per-build progress tool can
// import @opencode-ai/plugin without downloading anything at build time.
const configDir = "/home/agent/.config/opencode"

func main() {
	name := flag.String("name", "codegym-base", "snapshot name to create")
	ttl := flag.Duration("ttl", time.Hour, "how long the snapshot stays warm while it is created")
	keep := flag.Bool("keep", false, "keep the build sandbox instead of deleting it after promotion")
	flag.Parse()

	if os.Getenv("DAYTONA_API_KEY") == "" {
		log.Fatal("DAYTONA_API_KEY not set — run via: doppler run -p codegym -c dev -- go run ./base-snapshot")
	}
	ctx := context.Background()
	client, err := daytona.NewClient()
	if err != nil {
		log.Fatalf("client init: %v", err)
	}
	defer client.Close(ctx)

	// A build sandbox is promoted, so it must not be ephemeral.
	sandbox, err := client.Create(ctx, types.SnapshotParams{
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:    map[string]string{"codegym": "base-snapshot"},
			Ephemeral: false,
		},
	})
	if err != nil {
		log.Fatalf("create build sandbox: %v", err)
	}
	fmt.Printf("base-snapshot: leased sandbox %s\n", sandbox.ID)

	steps := []struct {
		name    string
		command string
		timeout time.Duration
	}{
		{
			name:    "install JDK, Maven, Node, Python",
			timeout: 20 * time.Minute,
			command: "export DEBIAN_FRONTEND=noninteractive && " +
				"sudo apt-get update -y && sudo apt-get install -y --no-install-recommends " +
				"openjdk-21-jdk-headless maven nodejs npm python3 python3-pip && " +
				"sudo rm -rf /var/lib/apt/lists/*",
		},
		{
			name:    "install Go",
			timeout: 10 * time.Minute,
			command: "set -e; " +
				"ARCH=$(uname -m); " +
				"case \"$ARCH\" in x86_64) GOARCH=amd64;; aarch64) GOARCH=arm64;; *) echo \"unsupported arch $ARCH\" >&2; exit 1;; esac; " +
				"curl -fsSL \"https://go.dev/dl/go1.22.5.linux-${GOARCH}.tar.gz\" -o /tmp/go.tar.gz && " +
				"sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz && " +
				"sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go && " +
				"sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt",
		},
		{
			name:    "install the pinned opencode binary",
			timeout: 10 * time.Minute,
			command: "set -e; " +
				"ARCH=$(uname -m); " +
				"case \"$ARCH\" in x86_64) ASSET=opencode-linux-x64.tar.gz;; aarch64) ASSET=opencode-linux-arm64.tar.gz;; *) echo \"unsupported arch $ARCH\" >&2; exit 1;; esac; " +
				"curl -fsSL \"https://github.com/sst/opencode/releases/download/v" + opencodeVersion + "/${ASSET}\" -o /tmp/opencode.tar.gz && " +
				"sudo tar -xzf /tmp/opencode.tar.gz -C /usr/local/bin && rm /tmp/opencode.tar.gz && " +
				"sudo chmod +x /usr/local/bin/opencode",
		},
		{
			name:    "install the opencode plugin runtime",
			timeout: 10 * time.Minute,
			command: "set -e; " +
				"mkdir -p " + configDir + "/tools && cd " + configDir + " && " +
				"npm init -y >/dev/null 2>&1 && " +
				"npm install --no-audit --no-fund @opencode-ai/plugin@" + opencodeVersion,
		},
	}

	promoted := false
	defer func() {
		if !*keep && !promoted {
			if err := sandbox.Delete(ctx); err != nil {
				log.Printf("base-snapshot: sandbox %s did not delete: %v", sandbox.ID, err)
			}
		}
	}()

	for _, s := range steps {
		if err := run(ctx, sandbox, s.name, s.command, s.timeout); err != nil {
			if !*keep {
				_ = sandbox.Delete(ctx)
			}
			log.Fatalf("base-snapshot: %s: %v", s.name, err)
		}
	}

	fmt.Println("base-snapshot: verifying the installed toolchain")
	verify := "set -e; " +
		"java -version 2>&1 | head -n1; " +
		"mvn -v 2>&1 | head -n1; " +
		"node --version; " +
		"npm --version; " +
		"go version; " +
		"python3 --version; " +
		"opencode --version; " +
		"cd " + configDir + " && node -e \"require.resolve('@opencode-ai/plugin'); console.log('plugin-runtime ok')\""
	if err := run(ctx, sandbox, "verify toolchain", verify, 5*time.Minute); err != nil {
		log.Fatalf("base-snapshot: verification failed: %v", err)
	}

	fmt.Printf("base-snapshot: promoting snapshot %q (ttl %s)\n", *name, *ttl)
	if err := sandbox.ExperimentalCreateSnapshotWithTimeout(ctx, *name, *ttl); err != nil {
		log.Fatalf("base-snapshot: promote: %v", err)
	}
	promoted = true

	fmt.Println()
	fmt.Printf("base-snapshot: created %q\n", *name)
	fmt.Printf("base-snapshot: set CODEGYM_BASE_SNAPSHOT=%s and keep it in the deploy secrets\n", *name)
	if *keep {
		fmt.Printf("base-snapshot: build sandbox %s kept for inspection\n", sandbox.ID)
	} else {
		if err := sandbox.Delete(ctx); err != nil {
			log.Printf("base-snapshot: sandbox %s did not delete: %v", sandbox.ID, err)
		}
	}
}

// run executes one install step, streaming its output and failing on a
// nonzero exit so a broken toolchain never gets promoted.
func run(ctx context.Context, sandbox *daytona.Sandbox, name, command string, timeout time.Duration) error {
	fmt.Printf("  → %s\n", name)
	start := time.Now()
	res, err := sandbox.Process.ExecuteCommand(ctx, command, options.WithExecuteTimeout(timeout))
	if err != nil {
		return err
	}
	output := strings.TrimRight(res.Result, "\n")
	if output != "" {
		for _, line := range strings.Split(output, "\n") {
			fmt.Println("    " + line)
		}
	}
	fmt.Printf("    (%s, exit %d)\n", time.Since(start).Round(time.Second), res.ExitCode)
	if res.ExitCode != 0 {
		return fmt.Errorf("exit code %d", res.ExitCode)
	}
	return nil
}
