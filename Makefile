
# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:crdVersions=v1"
TEST_PROFILE ?= $(CURDIR)/profile.cov
LINT_VERSION = 1.54.0
# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SED ?= sed -i
ifeq ($(shell go env GOOS),darwin)
SED = sed -i ''
endif
TEST_PACKAGES=$(shell go list ./... | egrep "controllers|internal/utils|api|client" | egrep -v "client/sm/smfakes" | paste -sd " " -)

GO_TEST = go test $(TEST_PACKAGES) -coverprofile=$(TEST_PROFILE) -ginkgo.flakeAttempts=3

all: manager

# Run tests go test and coverage
test: generate fmt vet manifests
	KUBEBUILDER_ASSETS="$(shell setup-envtest use --bin-dir /usr/local/bin -p path)" $(GO_TEST)

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests helm-charts
	helm upgrade --install sap-btp-operator ./sapbtp-operator-charts \
        --create-namespace \
        --namespace=sap-btp-operator \
        --values=hack/override_values.yaml \
		--set manager.image.repository=${IMG} \
		--set manager.image.tag=latest

undeploy:
	helm uninstall sap-btp-operator -n sap-btp-operator

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build:
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

envtest:
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest


lint:
	@echo "Running golangci-lint"
	@echo "----------------------------------------"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run --skip-dirs "pkg/mod"

lint-deps:

helm-charts:
	kustomize build config/default > ./sapbtp-operator-charts/templates/crd.yml

precommit: goimports lint test ## Run this before commiting

goimports:
	go install golang.org/x/tools/cmd/goimports@latest
	goimports -l -w .
