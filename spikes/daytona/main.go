// Command daytona-spike validates Daytona as CodeGym's execution layer.
//
// It exercises the two planned tracks end to end against the real Daytona API:
//
//	Phase A — deterministic submission run: ephemeral sandbox with all network
//	          egress blocked, LeetCode-style Python solution + tests uploaded
//	          and executed, network lockdown verified from inside.
//	Phase B — agent-scaffold run: network-enabled sandbox where an "agent"
//	          scaffolds a Node/Express project (npm install against the real
//	          registry), starts the server, tests it from inside, and exposes
//	          it through a preview URL fetched from outside.
//	Phase C — (optional, -promote) promotes the scaffolded sandbox to a
//	          reusable snapshot, the hybrid "agent builds it once, everyone
//	          else gets the deterministic path" flow.
//
// Run with Doppler supplying DAYTONA_API_KEY / DAYTONA_API_URL:
//
//	doppler run -p codegym -c dev -- go run .
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

var timings []string

func step(name string, fn func() error) {
	start := time.Now()
	err := fn()
	elapsed := time.Since(start).Round(time.Millisecond)
	status := "ok"
	if err != nil {
		status = "FAIL: " + err.Error()
	}
	line := fmt.Sprintf("%-52s %10s  %s", name, elapsed, status)
	timings = append(timings, line)
	fmt.Println("  " + line)
	if err != nil {
		log.Fatalf("aborting after failed step %q", name)
	}
}

func main() {
	promote := flag.Bool("promote", false, "also promote the scaffolded sandbox to a snapshot (slow)")
	flag.Parse()

	if os.Getenv("DAYTONA_API_KEY") == "" {
		log.Fatal("DAYTONA_API_KEY not set — run via: doppler run -p codegym -c dev -- go run .")
	}

	ctx := context.Background()
	client, err := daytona.NewClient()
	if err != nil {
		log.Fatalf("client init: %v", err)
	}
	defer client.Close(ctx)

	phaseA(ctx, client)
	sandboxB := phaseB(ctx, client)
	if *promote {
		phaseC(ctx, client, sandboxB)
	}
	step("cleanup: delete agent sandbox", func() error {
		return sandboxB.Delete(ctx)
	})

	fmt.Println("\n=== timing summary ===")
	for _, t := range timings {
		fmt.Println(t)
	}
}

// phaseA proves the deterministic submission path: ephemeral, network-blocked
// sandbox running an uploaded solution against an uploaded test file.
func phaseA(ctx context.Context, client *daytona.Client) {
	fmt.Println("\n=== Phase A: deterministic submission run (network blocked, ephemeral) ===")

	var sb *daytona.Sandbox
	step("A1 create sandbox (default snapshot)", func() error {
		var err error
		sb, err = client.Create(ctx, types.SnapshotParams{
			SandboxBaseParams: types.SandboxBaseParams{
				Labels:          map[string]string{"codegym": "spike", "track": "deterministic"},
				NetworkBlockAll: true,
				Ephemeral:       true,
			},
		})
		return err
	})
	defer sb.Delete(ctx)

	solution := []byte(`def two_sum(nums, target):
    seen = {}
    for i, n in enumerate(nums):
        if target - n in seen:
            return [seen[target - n], i]
        seen[n] = i
    return []
`)
	tests := []byte(`import solution

cases = [
    (([2, 7, 11, 15], 9), [0, 1]),
    (([3, 2, 4], 6), [1, 2]),
    (([3, 3], 6), [0, 1]),
]
for args, want in cases:
    got = solution.two_sum(*args)
    assert got == want, f"two_sum{args}: got {got}, want {want}"
print(f"PASS {len(cases)} cases")
`)

	step("A2 upload solution + tests", func() error {
		if err := sb.FileSystem.CreateFolder(ctx, "work"); err != nil {
			return err
		}
		if err := sb.FileSystem.UploadFile(ctx, solution, "work/solution.py"); err != nil {
			return err
		}
		return sb.FileSystem.UploadFile(ctx, tests, "work/test_solution.py")
	})

	step("A3 run tests", func() error {
		res, err := sb.Process.ExecuteCommand(ctx, "cd ~/work && python3 test_solution.py",
			options.WithExecuteTimeout(30*time.Second),
		)
		if err != nil {
			return err
		}
		fmt.Printf("      output: %s", indent(res.Result))
		if res.ExitCode != 0 {
			return fmt.Errorf("tests exited %d", res.ExitCode)
		}
		return nil
	})

	step("A4 verify egress is blocked", func() error {
		res, err := sb.Process.ExecuteCommand(ctx,
			"curl -sS -m 8 https://registry.npmjs.org/ && echo REACHED || echo BLOCKED")
		if err != nil {
			return err
		}
		if strings.Contains(res.Result, "REACHED") {
			return fmt.Errorf("network NOT blocked — egress succeeded")
		}
		return nil
	})

	step("A5 delete submission sandbox", func() error {
		return sb.Delete(ctx)
	})
}

// phaseB proves the agent-scaffold path: a network-enabled sandbox where the
// steps an agent harness would take (write files, npm install, run server)
// all work, plus an externally reachable preview URL.
func phaseB(ctx context.Context, client *daytona.Client) *daytona.Sandbox {
	fmt.Println("\n=== Phase B: agent-scaffold run (network on, preview URL) ===")

	var sb *daytona.Sandbox
	step("B1 create sandbox (network enabled)", func() error {
		var err error
		sb, err = client.Create(ctx, types.SnapshotParams{
			SandboxBaseParams: types.SandboxBaseParams{
				Labels: map[string]string{"codegym": "spike", "track": "agent"},
			},
		})
		return err
	})

	step("B2 toolchain present (node/npm)", func() error {
		res, err := sb.Process.ExecuteCommand(ctx, "node --version && npm --version")
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("node/npm missing in default snapshot: %s", res.Result)
		}
		fmt.Printf("      %s", indent(res.Result))
		return nil
	})

	pkg := []byte(`{"name":"codegym-scaffold","private":true,"dependencies":{"express":"^4"}}`)
	server := []byte(`const express = require("express");
const app = express();
app.use(express.json());
app.post("/sum", (req, res) => res.json({ sum: req.body.a + req.body.b }));
app.get("/", (_req, res) => res.send("codegym scaffold up"));
app.listen(3000, () => console.log("listening on 3000"));
`)

	step("B3 scaffold project files", func() error {
		if err := sb.FileSystem.CreateFolder(ctx, "work/app"); err != nil {
			return err
		}
		if err := sb.FileSystem.UploadFile(ctx, pkg, "work/app/package.json"); err != nil {
			return err
		}
		return sb.FileSystem.UploadFile(ctx, server, "work/app/server.js")
	})

	step("B4 npm install (real registry egress)", func() error {
		res, err := sb.Process.ExecuteCommand(ctx, "cd ~/work/app && npm install --no-audit --no-fund",
			options.WithExecuteTimeout(3*time.Minute),
		)
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("npm install exited %d: %s", res.ExitCode, tail(res.Result, 400))
		}
		return nil
	})

	step("B5 start server + hit it from inside", func() error {
		res, err := sb.Process.ExecuteCommand(ctx,
			`cd ~/work/app && sh -c 'nohup node server.js >server.log 2>&1 & sleep 1; curl -s -X POST localhost:3000/sum -H "Content-Type: application/json" -d "{\"a\":2,\"b\":40}"'`,
			options.WithExecuteTimeout(30*time.Second),
		)
		if err != nil {
			return err
		}
		if !strings.Contains(res.Result, `"sum":42`) {
			return fmt.Errorf("unexpected response: %s", res.Result)
		}
		fmt.Printf("      response: %s", indent(res.Result))
		return nil
	})

	step("B6 preview URL reachable from outside", func() error {
		link, err := sb.GetPreviewLink(ctx, 3000)
		if err != nil {
			return err
		}
		fmt.Printf("      preview: %s\n", link.URL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-daytona-preview-token", link.Token)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "codegym scaffold up") {
			return fmt.Errorf("status %d, body %q", resp.StatusCode, tail(string(body), 200))
		}
		return nil
	})

	return sb
}

// phaseC promotes the scaffolded environment to a reusable snapshot, then
// boots a network-blocked deterministic sandbox from it — the hybrid flow
// where an agent-built runtime becomes a template for everyone else.
func phaseC(ctx context.Context, client *daytona.Client, sb *daytona.Sandbox) {
	fmt.Println("\n=== Phase C: promote scaffold to snapshot, reuse deterministically ===")
	name := fmt.Sprintf("codegym-spike-express-%d", time.Now().Unix())

	step("C1 create snapshot from scaffolded sandbox", func() error {
		return sb.ExperimentalCreateSnapshotWithTimeout(ctx, name, 10*time.Minute)
	})

	var det *daytona.Sandbox
	step("C2 boot network-blocked sandbox from snapshot", func() error {
		var err error
		det, err = client.Create(ctx, types.SnapshotParams{
			Snapshot: name,
			SandboxBaseParams: types.SandboxBaseParams{
				Labels:          map[string]string{"codegym": "spike", "track": "promoted"},
				NetworkBlockAll: true,
				Ephemeral:       true,
			},
		})
		return err
	})
	defer det.Delete(ctx)

	step("C3 deps pre-baked, server runs with no egress", func() error {
		res, err := det.Process.ExecuteCommand(ctx,
			`test -d ~/work/app/node_modules/express && cd ~/work/app && sh -c 'nohup node server.js >server.log 2>&1 & sleep 1; curl -s localhost:3000/'`,
			options.WithExecuteTimeout(30*time.Second),
		)
		if err != nil {
			return err
		}
		if !strings.Contains(res.Result, "codegym scaffold up") {
			return fmt.Errorf("unexpected: exit %d, %s", res.ExitCode, tail(res.Result, 300))
		}
		fmt.Printf("      response: %s", indent(res.Result))
		return nil
	})

	step("C4 cleanup: delete promoted sandbox + snapshot", func() error {
		if err := det.Delete(ctx); err != nil {
			return err
		}
		snap, err := client.Snapshot.Get(ctx, name)
		if err != nil {
			return err
		}
		return client.Snapshot.Delete(ctx, snap)
	})
}

func indent(s string) string {
	s = strings.TrimRight(s, "\n") + "\n"
	return strings.ReplaceAll(s, "\n", "\n      ")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
