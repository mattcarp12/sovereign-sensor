# Contributing to Sovereign Sensor

Thanks for your interest in contributing. This document covers everything you need to get a change from idea to merged PR.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Ways to Contribute](#ways-to-contribute)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Submitting a PR](#submitting-a-pr)
- [Architecture Notes](#architecture-notes)

---

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). By participating, you agree to abide by its terms.

---

## Ways to Contribute

- **Bug reports** — open an issue with the `bug` label. Include your Kubernetes version, Tetragon version, and steps to reproduce.
- **Feature requests** — open an issue with the `enhancement` label before writing code. A quick discussion up front avoids wasted effort.
- **Documentation** — typo fixes, clarifications, and new guides are always welcome and don't require an issue first.
- **Code** — see below.

If you're unsure where to start, look for issues tagged [`good first issue`](../../issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).

---

## Development Setup

**Prerequisites**

- Go 1.22+
- Docker (or Podman — set `CONTAINER_TOOL=podman`)
- [`kind`](https://kind.sigs.k8s.io/)
- `kubectl`
- `make`

**Spin up a full local environment**

```bash
make dev
```

This single command generates CRDs, vendors Tetragon manifests, builds both container images, creates a Kind cluster, deploys the stack, and applies sample resources. When it finishes, the React dashboard is available at `http://localhost:8080`.

**Useful individual targets**

```bash
make test          # Unit tests (controller, policy evaluator, GeoIP, API server)
make lint          # golangci-lint with Kubernetes logging conventions plugin
make lint-fix      # Auto-fix lint issues where possible
make test-e2e      # Full end-to-end tests in an isolated Kind cluster
```

Run `make help` for the full list.

---

## Making Changes

### Branching

Fork the repo, then work on a branch named `<type>/<short-description>`, e.g.:

```
fix/tracing-policy-ip-append
feat/sse-violation-stream
docs/contributing-guide
```

### CRD and API changes

The CRD schemas in `config/crd/bases/` and the RBAC in `config/rbac/` are **generated** — do not edit them directly. Make changes to the types in `api/v1alpha1/` and then run:

```bash
make manifests   # Regenerates CRDs and RBAC from kubebuilder markers
make generate    # Regenerates DeepCopy methods
```

Commit the generated files alongside your type changes.

### Controller changes

The reconciler is in `internal/controller/sovereigntypolicy_controller.go`. A few conventions to keep in mind:

- The controller is the **sole writer** of the `TracingPolicy` resource. The agent reports facts (violator IPs) to the CRD status; the controller decides what gets enforced in the kernel.
- Reconciliation must be idempotent. Every code path through `Reconcile` should produce the same result if called twice with the same inputs.
- Use `log.FromContext(ctx)` for all logging. Follow [Kubernetes logging conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#message-style-guidelines) — structured key-value pairs, no string formatting in log messages.

### Agent changes

The agent lives in `cmd/agent/` and `internal/`. It runs as a DaemonSet and processes Tetragon gRPC events. Key packages:

| Package | Responsibility |
|---|---|
| `internal/k8s/watcher.go` | Consumes the Tetragon event stream |
| `internal/policy/evaluator.go` | Matches events against active policies |
| `internal/policy/reporter.go` | Appends violator IPs to CRD status |
| `internal/geo/geoip.go` | MaxMind GeoLite2 country lookup |

### Frontend changes

The React dashboard is in `frontend/`. It's built and embedded into the controller binary at compile time via `make build-frontend`.

```bash
cd frontend && npm install && npm run dev   # Hot-reload dev server
```

Note that the dev server proxies API calls to `localhost:8080`, so you'll need `make dev` running in another terminal.

---

## Testing

Every code change should include tests. The project has three layers:

**Unit tests** (`internal/...`) — fast, no cluster required. Run with `make test`. Add tests alongside the code you're changing; the evaluator and GeoIP packages have good examples to follow.

**Controller tests** (`internal/controller/...`) — use [envtest](https://book.kubebuilder.io/reference/envtest.html), which spins up a real API server and etcd locally. No cluster or Docker needed. Also run with `make test`.

**End-to-end tests** (`test/e2e/`) — spin up a Kind cluster, build and load images, and exercise the full enforcement path including Tetragon. Run with `make test-e2e`. These are slower and run in CI on every PR; you don't need to run them locally before every commit, but please run them before opening a PR that touches the agent or controller.

The project uses [Ginkgo](https://onsi.github.io/ginkgo/) for BDD-style tests. Follow the existing patterns in `*_test.go` files.

---

## Submitting a PR

1. Make sure `make test` and `make lint` pass locally.
2. Keep PRs focused — one logical change per PR makes review faster.
3. Write a clear PR description: what changed, why, and how to verify it. Link to the related issue if one exists.
4. The PR title should follow [Conventional Commits](https://www.conventionalcommits.org/) format: `fix:`, `feat:`, `docs:`, `chore:`, etc. This feeds the release changelog automatically once GitHub Actions are in place.
5. A maintainer will review within a few days. Small, well-scoped PRs get reviewed faster.

**DCO sign-off**

All commits must be signed off in accordance with the [Developer Certificate of Origin](https://developercertificate.org/). Add `-s` to your commit command:

```bash
git commit -s -m "fix: append IPs to TracingPolicy instead of overwriting"
```

