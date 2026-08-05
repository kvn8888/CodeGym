package execution

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

const goSnapshotCreateSamples = 15

// TestMeasureGoSnapshotCreateLatency is the opt-in promotion decision gate for
// a Go snapshot candidate. It alternates v2 and v3 on every create, performs a
// real upload/readiness command, and deletes each sandbox before the next pair.
func TestMeasureGoSnapshotCreateLatency(t *testing.T) {
	if os.Getenv("CODEGYM_MEASURE_GO_SNAPSHOT") != "1" {
		t.Skip("set CODEGYM_MEASURE_GO_SNAPSHOT=1 to run the interleaved v2/v3 create gate")
	}
	client := daytonaTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	snapshots := []string{GoSnapshotV2Name, GoSnapshotCandidateName}
	measurements := map[string][]int64{
		GoSnapshotV2Name:        make([]int64, 0, goSnapshotCreateSamples),
		GoSnapshotCandidateName: make([]int64, 0, goSnapshotCreateSamples),
	}
	for sample := 1; sample <= goSnapshotCreateSamples; sample++ {
		for _, snapshotName := range snapshots {
			createMs := measureSnapshotCreate(t, ctx, client, snapshotName, sample)
			measurements[snapshotName] = append(measurements[snapshotName], createMs)
			t.Logf("snapshot_create_sample snapshot=%s sample=%02d create_ms=%d", snapshotName, sample, createMs)
		}
	}

	for _, snapshotName := range snapshots {
		snapshot, err := client.Snapshot.Get(ctx, snapshotName)
		if err != nil {
			t.Fatalf("get snapshot %s metadata: %v", snapshotName, err)
		}
		size := "unknown"
		if snapshot.Size != nil {
			size = fmt.Sprintf("%.6f", *snapshot.Size)
		}
		median, p90, maximum := summarizeMilliseconds(measurements[snapshotName])
		t.Logf("snapshot_create_summary snapshot=%s n=%d median_ms=%d p90_ms=%d max_ms=%d reported_size=%s",
			snapshotName, len(measurements[snapshotName]), median, p90, maximum, size)
	}
}

func measureSnapshotCreate(
	t *testing.T,
	ctx context.Context,
	client *daytona.Client,
	snapshotName string,
	sample int,
) int64 {
	t.Helper()
	started := time.Now()
	sandbox, err := client.Create(ctx, types.SnapshotParams{
		Snapshot: snapshotName,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels: map[string]string{
				"codegym": "snapshot-measure", "track": "v2-v3-interleaved",
				"snapshot": snapshotName, "sample": fmt.Sprintf("%02d", sample),
			},
			NetworkBlockAll: true,
			Ephemeral:       true,
		},
	})
	createMs := time.Since(started).Milliseconds()
	if err != nil {
		t.Fatalf("create snapshot=%s sample=%d: %v", snapshotName, sample, err)
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := sandbox.Delete(cleanupCtx); err != nil {
			t.Fatalf("delete snapshot=%s sample=%d: %v", snapshotName, sample, err)
		}
	}
	if err := sandbox.FileSystem.CreateFolder(ctx, "work"); err != nil {
		cleanup()
		t.Fatalf("create readiness directory snapshot=%s sample=%d: %v", snapshotName, sample, err)
	}
	if err := sandbox.FileSystem.UploadFile(ctx, []byte("ready\n"), "work/ready.txt"); err != nil {
		cleanup()
		t.Fatalf("upload readiness file snapshot=%s sample=%d: %v", snapshotName, sample, err)
	}
	result, err := sandbox.Process.ExecuteCommand(ctx, "test -s ~/work/ready.txt", options.WithExecuteTimeout(15*time.Second))
	cleanup()
	if err != nil {
		t.Fatalf("readiness command snapshot=%s sample=%d: %v", snapshotName, sample, err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("readiness command snapshot=%s sample=%d exited %d: %s", snapshotName, sample, result.ExitCode, result.Result)
	}
	return createMs
}

func summarizeMilliseconds(values []int64) (median, p90, maximum int64) {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if len(sorted) == 0 {
		return 0, 0, 0
	}
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		median = (sorted[middle-1] + sorted[middle]) / 2
	} else {
		median = sorted[middle]
	}
	p90Index := int(math.Ceil(0.9*float64(len(sorted)))) - 1
	return median, sorted[p90Index], sorted[len(sorted)-1]
}
