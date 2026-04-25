package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type AgentEvent struct {
	DstIP      string `json:"dest_ip"`
	DstCountry string `json:"dest_country"`
	Pod        string `json:"pod_name"`
	Namespace  string `json:"namespace"`
	Verdict    struct {
		Action string `json:"action"`
		Policy string `json:"policy_name"`
	} `json:"verdict"`
}

func TestEgressSovereignty(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode.")
	}

	// Ensure kubectl exists
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Fatalf("kubectl not found in PATH: %v", err)
	}

	// Ensure we can talk to cluster
	runCmdStrict(t, "kubectl", "cluster-info")

	// 1. Create namespace
	runCmd(t, "kubectl", "create", "namespace", "eu-prod")
	defer runCmd(t, "kubectl", "delete", "namespace", "eu-prod")

	// 2. Apply the test policy
	// The policy file should exist in test/test-policy.yaml
	policyPath := filepath.Join("test-policy.yaml")
	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("missing test-policy.yaml: %v", err)
	}

	runCmdStrict(t, "kubectl", "apply", "-f", policyPath)
	defer runCmd(t, "kubectl", "delete", "-f", policyPath)

	// 3. Start the agent in the background and capture stdout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentLogsCmd := exec.CommandContext(ctx, "kubectl", "logs", "-n", "kube-system", "-l", "app=sovereign-sensor", "-f")
	agentLogsCmd.Stderr = os.Stderr

	stdoutPipe, err := agentLogsCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	if err := agentLogsCmd.Start(); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}

	// Make sure we kill the agent when test finishes
	defer func() {
		cancel()
		_ = agentLogsCmd.Wait()
	}()

	// Start scanning agent output
	eventCh := make(chan AgentEvent, 50)
	go scanAgentOutput(t, stdoutPipe, eventCh)

	// Wait for agent to initialize
	time.Sleep(4 * time.Second)

	// 4. Trigger a violation
	t.Log("Triggering violation: Curling Datadog from eu-prod...")

	runCmdStrict(t,
		"kubectl", "run", "violator",
		"-n", "eu-prod",
		"--image=alpine",
		"--restart=Never",
		"--",
		"sh", "-c",
		"apk add --no-cache curl && curl -s https://api.datadoghq.com || true",
	)

	defer runCmd(t, "kubectl", "delete", "pod", "violator", "-n", "eu-prod")

	// 5. Assert agent output contains a "block" event
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for agent to emit a block event")

		case evt := <-eventCh:
			if strings.ToLower(evt.Verdict.Action) == "block" {
				t.Log("SUCCESS: received expected block event")
				return
			}
		}
	}
}

func scanAgentOutput(t *testing.T, stdoutPipe io.ReadCloser, eventCh chan<- AgentEvent) {
	t.Helper()

	reader := bufio.NewScanner(stdoutPipe)

	for reader.Scan() {
		line := reader.Text()

		// Ignore non-json log lines
		if !strings.HasPrefix(strings.TrimSpace(line), "{") {
			continue
		}

		var evt AgentEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}

		eventCh <- evt
	}

	if err := reader.Err(); err != nil {
		t.Logf("agent stdout scanner error: %v", err)
	}
}

func runCmd(t *testing.T, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: %s %v\nOutput:\n%s\nErr: %v",
			name, args, string(out), err)
	}
}

func runCmdStrict(t *testing.T, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %s %v\nOutput:\n%s\nErr: %v",
			name, args, string(out), err)
	}
	fmt.Printf("%s", out)
}
