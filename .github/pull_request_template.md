## What does this PR do?

<!-- A concise description of the change. Link to the issue it closes if one exists. -->

Closes #

## Type of change

<!-- Check all that apply -->

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor (no behavior change)
- [ ] Documentation
- [ ] CI / tooling
- [ ] CRD / API change (requires `make manifests generate`)

## How was this tested?

<!-- Describe how you verified the change works. For bug fixes, explain how you confirmed the bug is gone.
     For features, describe the scenario you tested. Paste relevant log output or kubectl output if helpful. -->

## Checklist

- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] New or modified behavior has test coverage
- [ ] CRD or RBAC changes were regenerated with `make manifests generate` and the generated files are committed
- [ ] Commits are signed off (`git commit -s`) per the [DCO](https://developercertificate.org/)

## For CRD / API changes only

- [ ] The change is backwards-compatible with existing `SovereigntyPolicy` and `SovereignSensor` resources, OR a migration path is documented below

<!-- If this is a breaking API change, describe what existing users need to do: -->
