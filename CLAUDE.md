# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Does

This is a Kubernetes operator that lets clusters consume SAP BTP services via Kubernetes-native CRDs. It talks to **SAP Service Manager** (via the Open Service Broker API) to provision `ServiceInstance` and `ServiceBinding` resources, then writes credentials as Kubernetes Secrets.

## Commands

```bash
# Build
make manager            # builds bin/manager
go build -o bin/manager main.go

# Test (requires setup-envtest)
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
make test               # runs generate, fmt, vet, manifests, then tests

# Run a single test package
KUBEBUILDER_ASSETS="$(setup-envtest use --bin-dir /usr/local/bin -p path)" \
  go test ./controllers/... -ginkgo.flakeAttempts=3

# Run a specific Ginkgo test by name
KUBEBUILDER_ASSETS="$(setup-envtest use --bin-dir /usr/local/bin -p path)" \
  go test ./controllers/... --ginkgo.focus="<test description>"

# Lint
golangci-lint run --skip-dirs "pkg/mod"

# Format + imports
go fmt ./...
goimports -l -w .

# Regenerate CRD manifests and deepcopy after modifying *_types.go
make manifests generate

# Pre-commit (goimports + lint + test)
make precommit

# Local deploy (needs kind + credentials in hack/override_values.yaml)
make docker-build
kind load docker-image controller:latest
make deploy
```

## Architecture

### Controllers (`controllers/`)

Three reconcilers, all wired up in `main.go`:

- **`ServiceInstanceReconciler`** — provisions/updates/deletes service instances in SAP Service Manager. Manages a finalizer for cleanup ordering.
- **`ServiceBindingReconciler`** — creates bindings against a provisioned instance, then writes the broker-returned credentials to a Kubernetes Secret. Also handles credential rotation.
- **`SecretReconciler`** — watches labeled Secrets (`services.cloud.sap.com/watch-secret: "true"`) and re-triggers reconciliation of any `ServiceInstance` that references them via label `services.cloud.sap.com/secret-ref_<name>`. This makes instance parameters from secrets reactive.

### API Types (`api/`)

- `api/v1/` — primary CRD types: `ServiceInstance`, `ServiceBinding`. Each has a `Spec`, a `Status` with Kubernetes-standard `Conditions`, and validating + mutating webhooks.
- `api/common/` — shared interfaces (`SAPBTPResource`), condition constants (`ConditionReady`, `ConditionSucceeded`, `ConditionFailed`), label/annotation constants, and `api/common/utils/secret_template.go` which handles Go-template-based secret rendering using a subset of sprig functions.

### SM Client (`client/sm/`)

`sm.Client` is the interface to SAP Service Manager. `smfakes/` contains a generated counterfeiter fake used in controller tests — the test suite injects `fakeClient` directly instead of hitting a real SM. The client handles OAuth2 (client credentials or mTLS) via `internal/auth/`.

### Credential Secret Resolution

When a `ServiceInstance` is reconciled, `GetSMClient` looks up the SM credentials in this priority order:
1. `spec.btpAccessCredentialsSecret` (explicit override, read from management namespace)
2. Secret named `sap-btp-service-operator` in the instance's own namespace
3. Secret named `<namespace>-sap-btp-service-operator` in the management namespace
4. Secret named `sap-btp-service-operator` in the release namespace (cluster default)

### Testing

Controller tests use `envtest` (real API server + etcd, no cluster needed) with a `smfakes.FakeClient` standing in for SAP Service Manager. Tests are Ginkgo BDD-style. `setup-envtest` must be installed and CRD manifests in `config/crd/bases/` must be up-to-date before running tests.

### Code Generation

After modifying `*_types.go` files, run `make manifests generate` to regenerate:
- CRD YAML in `config/crd/bases/`
- `zz_generated.deepcopy.go` in each API package

The `+kubebuilder:` marker comments in type files and controllers drive both outputs.
