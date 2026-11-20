// Command list-sandboxes prints Daytona sandboxes for debugging capacity
// issues. It is read-only: it never deletes or modifies a sandbox.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
)

func main() {
	cleanupBase := flag.Bool("cleanup-base", false, "delete leftover base-snapshot sandboxes")
	flag.Parse()
	ctx := context.Background()
	client, err := daytona.NewClient()
	if err != nil {
		log.Fatalf("client init: %v", err)
	}
	defer client.Close(ctx)

	for sandbox, err := range client.ListSeq(ctx, nil) {
		if err != nil {
			log.Fatalf("list sandboxes: %v", err)
		}
		fmt.Printf("id=%s snapshot=%v state=%s cpu=%.1f memory=%.0f labels=%v\n",
			sandbox.ID, sandbox.Snapshot, sandbox.State, sandbox.Cpu, sandbox.Memory, sandbox.Labels)
		if *cleanupBase && sandbox.Labels["codegym"] == "base-snapshot" {
			if err := sandbox.Delete(ctx); err != nil {
				log.Printf("delete sandbox %s: %v", sandbox.ID, err)
				continue
			}
			fmt.Printf("deleted %s\n", sandbox.ID)
		}
	}
}
