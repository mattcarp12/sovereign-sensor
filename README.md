# Sovereign Sensor

Data sovereignty is a hard compliance mandate, not a networking suggestion. Sovereign Sensor enforces geographic network boundaries at the Linux kernel level, silently dropping unauthorized cross-border packets before the TCP handshake even begins.

## Architecture

* **Control Plane:** A Go-based Kubernetes Operator that translates `SovereigntyPolicy` Custom Resources into dynamic eBPF blocklists.
* **Data Plane:** Powered by Cilium Tetragon. An on-node Agent intercepts `tcp_connect` syscalls, resolves GeoIP routing, and forcefully closes file descriptors (`ECONNREFUSED`) destined for unauthorized regions.
* **Telemetry:** A React dashboard proxying the Kubernetes API to visualize real-time packet drops on a global heat map.

## Quickstart

Spin up the local `kind` environment and enforce your first boundary:

```bash
# 1. Install the baseline eBPF data plane 
kubectl apply -f https://github.com/cilium/tetragon/releases/download/v1.1.0/tetragon.yaml

# 2. Compile the operator, build the agent, and boot the stack
make dev

# 3. Apply a policy to block outbound traffic to specific ISO country codes
kubectl apply -f config/samples/policy.yaml
```

*Requires `docker`, `kind`, and Go 1.22+.*

