# Sovereign Sensor Documentation

Sovereign Sensor is a Kubernetes network security control plane that manages and enforces data sovereignty policies. It leverages eBPF (via Tetragon integration) to monitor egress traffic and block network connections directed to disallowed geographic territories based on ISO Alpha-2 country codes.

---

## Architectural Components

The system is composed of four principal components:
1. **Custom Controller Manager (Operator):** Reconciles `SovereigntyPolicy` resources and operates the control loop.
2. **eBPF Sensor Agent:** Monitors kernel-level network socket events, evaluates policy contexts, and handles geo-IP lookups using MaxMind DB structures.
3. **API Server:** Embedded within the controller manager binary to expose data endpoints for active policies and logged violations.
4. **React Management Interface:** A single-page dashboard embedded directly into the API binary via Go `embed`, providing unified configuration auditing and telemetry log visibility.

---

## Custom Resource Definitions (CRD)

The control plane monitors custom resources under the API group `sec.sovereign.io/v1alpha1`.

### SovereigntyPolicy Schema

| Field | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `metadata.name` | String | Lowercase alphanumeric identifier and hyphens only. | Yes |
| `spec.namespaces` | Array [String] | List of target namespaces to enforce. Defaults to `default`. | Yes |
| `spec.disallowedCountries`| Array [String] | Target country codes formatted to exactly 2 characters (ISO Alpha-2). | Yes |
| `spec.actions` | Array [String] | Strategy applied upon matching a violation. Supported options: `block-kill`, `block-noconn`, `log`. | Yes |

### CRD Manifest Example

```yaml
apiVersion: sec.sovereign.io/v1alpha1
kind: SovereigntyPolicy
metadata:
  name: block-restricted-zones
spec:
  namespaces:
    - production
    - staging
  disallowedCountries:
    - RU
    - CN
  actions:
    - block-kill
  description: Generated via Sovereign Sensor Web UI
  ```

---

## Local Development Setup

To run the development mesh locally on a workstation, ensure the following dependencies are installed globally:

* Docker (or Podman alternative)
* Go (v1.25+)
* Node.js & npm
* Kind (Kubernetes in Docker)
* Helm (v3+)
* Kubectl

### Step 1: Export Cluster Context

If a cluster already exists but context is missing inside your container or local environment terminal, link the context configuration:

```bash
kind export kubeconfig --name sovereign-test
kubectl config use-context kind-sovereign-test

```

### Step 2: Bootstrapping the Environment

Execute the optimized integration macro to generate artifacts, spin up Tetragon routing rules, deploy the manager, and spin up sample deployment assets:

```bash
make dev

```

Upon successful container initialization and pod status rollouts, the workspace automatically initiates a port forward. The interface is available locally via:
👉 `http://localhost:8080`

---

## Production Helm Deployment

The operator package is maintained as a standard Helm application template under `charts/sovereign-sensor`.

### Configuration Defaults (`values.yaml`)

```yaml
controllerManager:
  image:
    repository: ghcr.io/mattcarp12/sovereign-sensor-controller
    tag: latest

```

### Installation Target Execution

Deploy the sensor framework into the active cluster configuration using the localized chart directory:

```bash
helm install sovereign-sensor ./charts/sovereign-sensor \
  --namespace sovereign-sensor-system \
  --create-namespace
```

### RBAC Scope and Permissions

The chart initializes targeted access configurations via service accounts bound to specific API operations.

* **ClusterRole Permissions:** The manager possesses global authorization rights to `get`, `list`, and `watch` resources across the `sec.sovereign.io` API group.
* **Telemetry Binding:** The controller manager accesses core Kubernetes event loops to query records containing `FieldSelector: "reason=SovereigntyViolation"` across all cluster namespaces to populate dashboard arrays.
