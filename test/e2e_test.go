package main

import (
	"os/exec"
	"testing"
	"time"
)

// TestEgressSovereignty is a full integration test.
// It assumes `make env-up` has already been run and the cluster is ready.
func TestEgressSovereignty(t *testing.T) {
	// Skip this test during normal `go test ./...` runs unless explicitly requested
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode.")
	}

	// 1. Create the restricted namespace
	runCmd(t, "kubectl", "create", "namespace", "eu-prod")
	defer runCmd(t, "kubectl", "delete", "namespace", "eu-prod") // Cleanup after test

	// 2. Start the agent in the background (you would capture stdout here to assert the JSON)
	// ... (Agent execution logic) ...

	// Wait a second for the agent to connect to Tetragon
	time.Sleep(2 * time.Second)

	// 3. Trigger a network violation
	t.Log("Triggering violation: Curling Datadog from eu-prod...")
	err := exec.Command("kubectl", "run", "violator", "-n", "eu-prod", "--image=alpine", "--restart=Never", "--", "sh", "-c", "apk add curl && curl https://api.datadoghq.com").Run()
	if err != nil {
		t.Fatalf("Failed to trigger pod: %v", err)
	}
	defer runCmd(t, "kubectl", "delete", "pod", "violator", "-n", "eu-prod")

	// 4. Assert that the agent outputted a JSON payload with Action: "block"
	// ... (JSON parsing assertion logic) ...
}

// runCmd is a simple helper to execute shell commands
func runCmd(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Run(); err != nil {
		t.Logf("Command %s %v failed: %v (this might be expected during cleanup)", name, args, err)
	}
}
