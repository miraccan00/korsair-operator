# Load local dev environment variables from .env (gitignored, secrets safe).
# Copy .env.example → .env and fill in values for local development.
-include .env
export

# Auto-detect kubeconfig if not explicitly set in .env or the environment.
# Works for local ($HOME = /Users/<you>) and devcontainer ($HOME = /home/vscode).
KUBECONFIG ?= $(HOME)/.kube/config

# Image URL to use all building/pushing image targets
IMG ?= miraccan/korsair-operator:latest
# Frontend source directory
FRONTEND_DIR ?= frontend
# Embedded static files location (populated by build-frontend)
FRONTEND_DIST ?= cmd/web/static/dist

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Local Development

.PHONY: fast-up
fast-up: ## Deploy operator from DockerHub via Helm (miraccan/korsair-operator:latest).
	@echo "🚀 Fast startup: Helm deployment (miraccan/korsair-operator:latest)"
	@kind create cluster --name korsair-dev --wait 5m --image kindest/node:v1.32.0 2>&1 || echo "Cluster already exists"
	@kubectl config use-context kind-korsair-dev
	@helm repo add korsair . --force-update 2>/dev/null || true
	@helm upgrade --install korsair ./charts/korsair \
		--create-namespace \
		--namespace korsair-system \
		--wait \
		--timeout 3m
	@echo "✅ Deployment complete!"
	@echo "📊 Check status: kubectl -n korsair-system get pods"
	@echo "📋 View logs:   kubectl -n korsair-system logs -f -l app.kubernetes.io/component=operator"

.PHONY: fast-up-with-api-ui
fast-up-with-api-ui: ## Deploy operator + API + UI from DockerHub.
	@echo "🚀 Fast startup: Full stack (operator + API + UI)"
	@kind create cluster --name korsair-dev --wait 5m --image kindest/node:v1.32.0 2>&1 || echo "Cluster already exists"
	@kubectl config use-context kind-korsair-dev
	@helm repo add korsair . --force-update 2>/dev/null || true
	@helm upgrade --install korsair ./charts/korsair \
		--create-namespace \
		--namespace korsair-system \
		--set api.enabled=true \
		--set ui.enabled=true \
		--wait \
		--timeout 3m
	@echo "✅ Full stack deployed!"
	@echo "📊 Check status: kubectl -n korsair-system get pods"
	@echo "📋 View logs:   kubectl -n korsair-system logs -f -l app.kubernetes.io/component=operator"

.PHONY: fast-up-local
fast-up-local: ## Dev mode: Build image locally, load into Kind, deploy via Helm.
	@echo "🚀 Fast startup (LOCAL BUILD): Kind cluster + Helm deployment"
	@kind create cluster --name korsair-dev --wait 5m --image kindest/node:v1.32.0 2>&1 || echo "Cluster already exists"
	@echo "🐳 Building Docker image (controller:latest)..."
	@$(CONTAINER_TOOL) build -t controller:latest . --progress=plain
	@echo "📥 Loading image into Kind cluster..."
	@kind load docker-image controller:latest --name korsair-dev
	@echo "🎯 Installing Helm chart..."
	@kubectl config use-context kind-korsair-dev
	@helm repo add korsair . --force-update 2>/dev/null || true
	@helm upgrade --install korsair ./charts/korsair \
		--create-namespace \
		--namespace korsair-system \
		--set operator.image.repository=controller \
		--set operator.image.tag=latest \
		--set operator.image.pullPolicy=Never \
		--wait \
		--timeout 3m
	@echo "✅ Deployment complete!"
	@echo "📊 Check status: kubectl -n korsair-system get pods"
	@echo "📋 View logs:   kubectl -n korsair-system logs -f -l app.kubernetes.io/component=operator"

.PHONY: dev-setup
dev-setup: ## Create a local Kind cluster, build all images, and deploy Korsair end-to-end.
	bash hack/setup-kind.sh

.PHONY: dev-teardown
dev-teardown: ## Remove all Korsair resources and delete the local Kind cluster.
	bash hack/cleanup-test-cluster.sh

.PHONY: diagnose
diagnose: ## Debug fast-up issues step by step
	bash hack/diagnose.sh

.PHONY: cleanup-kind
cleanup-kind: ## Clean up existing kind clusters and create a fresh one
	bash hack/cleanup-kind.sh

.PHONY: fast-down
fast-down: ## Delete the Kind cluster created by fast-up.
	kind delete cluster --name korsair-dev || echo "Cluster 'korsair-dev' not found"

.PHONY: tilt-up
tilt-up: ## Start Tilt development environment (auto-reload, logs, tests).
	@command -v tilt >/dev/null 2>&1 || { echo "❌ tilt is not installed. Install from https://docs.tilt.dev/tutorial.html"; exit 1; }
	@echo "🚀 Starting Tilt development environment..."
	@echo "📊 Web UI will open at http://localhost:10350"
	tilt up

.PHONY: tilt-down
tilt-down: ## Stop Tilt environment and cleanup resources.
	tilt down

.PHONY: tilt-logs
tilt-logs: ## Stream logs from all resources via Tilt (CLI mode).
	tilt logs -f

.PHONY: tilt-debug
tilt-debug: ## Show Tilt environment status and debug info.
	@echo "🔍 Tilt Status:"
	@tilt status
	@echo ""
	@echo "📊 Kind Cluster Info:"
	@kind get clusters || echo "No kind clusters found"
	@echo ""
	@echo "🎯 Korsair Resources:"
	@kubectl get pods,imagescanjobs,securityscanconfigs -A 2>/dev/null | grep -E "(korsair|security)" || echo "No resources found yet"

.PHONY: test-smoke
test-smoke: ## Smoke test: apply the sample SecurityScanConfig and verify an ImageScanJob is created within 60s.
	@echo "Applying sample SecurityScanConfig..."
	kubectl apply -f config/samples/security_v1alpha1_securityscanconfig.yaml
	@echo "Waiting up to 60s for an ImageScanJob to appear..."
	@timeout 60 bash -c 'until kubectl get imagescanjobs -A 2>/dev/null | grep -q "."; do sleep 3; done' \
		&& echo "OK: ImageScanJob created successfully." \
		|| (echo "FAIL: No ImageScanJob appeared within 60s. Operator logs:"; \
		    kubectl logs -n korsair-system -l app.kubernetes.io/component=operator --tail=40; exit 1)

.PHONY: ci
ci: fmt vet lint test ## Run all CI checks: fmt, vet, lint, unit tests.

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="{./api/...,./cmd/...,./internal/...}" output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="{./api/...,./cmd/...,./internal/...}"

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= korsair-operator-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

# ── Docker Build Optimization ────────────────────────────────────────────
# Docker always uses .dockerignore (Docker doesn't support per-Dockerfile ignore files).
# Context exclusions are defined in:
#   .dockerignore — Used for all builds (Dockerfile, Dockerfile.api, Dockerfile.ui)
#                  Excludes: frontend/*, cmd/*, internal/*, test/*, docs/*, etc.
#
# For fine-grained context optimization in local dev:
#   make tilt-up  → Uses Tiltfile with per-resource 'only' filters
#                  (More efficient: operator build skips frontend/, UI build skips go.*)
#
# For CI/CD parallelized builds, the GitHub Actions workflow handles
# multi-platform builds across operator, API, and UI concurrently with GHA caching.

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host (env vars loaded from .env).
	go run ./cmd/main.go --metrics-secure=false --health-probe-bind-address=:8082

.PHONY: registry-secret
registry-secret: ## Create/update korsair-registry-credentials Secret from .env (BSO_REGISTRY_SERVER/USERNAME/PASSWORD).
	@if [ -z "$(BSO_REGISTRY_USERNAME)" ] || [ -z "$(BSO_REGISTRY_PASSWORD)" ]; then \
		echo "ERROR: BSO_REGISTRY_USERNAME and BSO_REGISTRY_PASSWORD must be set in .env"; exit 1; \
	fi
	kubectl create namespace korsair-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret docker-registry korsair-registry-credentials \
		--namespace=korsair-system \
		--docker-server=$(BSO_REGISTRY_SERVER) \
		--docker-username=$(BSO_REGISTRY_USERNAME) \
		--docker-password=$(BSO_REGISTRY_PASSWORD) \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "Secret korsair-registry-credentials updated for $(BSO_REGISTRY_SERVER)"

# All Dockerfiles use BuildKit cache mounts (--mount=type=cache) so incremental
# rebuilds are fast without any extra flags. Set DOCKER_BUILDKIT=1 if your
# Docker version does not enable BuildKit by default (Docker < 23).
#
# Target runtime-source (default): full in-container build — used for CI and releases.
# Target runtime-prebuilt:         copies a host-built binary — used by Tilt for dev.
#
# To build for a different platform: docker build --platform linux/arm64 ...
.PHONY: docker-build
docker-build: ## Build operator image from source (Dockerfile, target: runtime-source).
	$(CONTAINER_TOOL) build --target runtime-source -t ${IMG} .

.PHONY: docker-build-api
docker-build-api: ## Build API image from source (Dockerfile.api, target: runtime-source).
	$(CONTAINER_TOOL) build --target runtime-source -f Dockerfile.api -t $$(echo ${IMG} | sed 's/:/:api-/') .

.PHONY: docker-build-ui
docker-build-ui: ## Build UI image (Dockerfile.ui).
	$(CONTAINER_TOOL) build -f Dockerfile.ui -t $$(echo ${IMG} | sed 's/:/:ui-/') .

.PHONY: docker-build-all
docker-build-all: ## Build all images (operator, API, UI) in parallel.
	@echo "🚀 Building operator, API, and UI images concurrently..."
	@mkdir -p .build-logs
	@($(CONTAINER_TOOL) build --target runtime-source -t ${IMG} . > .build-logs/operator.log 2>&1 && echo "✅ Operator") & \
	 ($(CONTAINER_TOOL) build --target runtime-source -f Dockerfile.api -t $$(echo ${IMG} | sed 's/:/:api-/') . > .build-logs/api.log 2>&1 && echo "✅ API") & \
	 ($(CONTAINER_TOOL) build -f Dockerfile.ui -t $$(echo ${IMG} | sed 's/:/:ui-/') . > .build-logs/ui.log 2>&1 && echo "✅ UI") & \
	 wait
	@echo "🎉 All images built. Logs: .build-logs/{operator,api,ui}.log"

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-push-all
docker-push-all: ## Push all images (operator, API, UI) in parallel.
	@echo "📤 Pushing operator, API, and UI images concurrently..."
	@($(CONTAINER_TOOL) push ${IMG} > /dev/null 2>&1 && echo "✅ Operator pushed") & \
	 ($(CONTAINER_TOOL) push $$(echo ${IMG} | sed 's/:/:api-/') > /dev/null 2>&1 && echo "✅ API pushed") & \
	 ($(CONTAINER_TOOL) push $$(echo ${IMG} | sed 's/:/:ui-/') > /dev/null 2>&1 && echo "✅ UI pushed") & \
	 wait
	@echo "🎉 All images pushed successfully!"

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name korsair-operator-builder
	$(CONTAINER_TOOL) buildx use korsair-operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm korsair-operator-builder
	rm Dockerfile.cross

##@ Web Dashboard

.PHONY: build-frontend
build-frontend: ## Build React frontend (pnpm install + pnpm run build → cmd/web/static/dist/).
	cd "$(FRONTEND_DIR)" && corepack enable && pnpm install --frozen-lockfile
	cd "$(FRONTEND_DIR)" && pnpm run build
	mkdir -p "$(FRONTEND_DIST)"
	cp -r "$(FRONTEND_DIR)/dist/." "$(FRONTEND_DIST)/"

.PHONY: run-api
run-api: ## Run API-only backend for local dev (no SPA embed; use 'pnpm run dev' for frontend).
	@echo "Starting BSO API backend on :8090 (API-only mode)..."
	@echo "  → Run 'cd frontend && pnpm run dev' in another terminal for the frontend (Vite proxies /api here)"
	go run ./cmd/web/ --listen-addr=:8090

##@ Helm

.PHONY: helm-crds
helm-crds: manifests ## Sync generated CRDs into Helm chart templates/crds/.
	cp config/crd/bases/security.blacksyrius.com_imagescanjobs.yaml \
	   charts/bso/templates/crds/imagescanjobs.yaml.tmp && \
	{ printf '{{- if .Values.installCRDs }}\n'; cat charts/bso/templates/crds/imagescanjobs.yaml.tmp; printf '\n{{- end }}\n'; } \
	   > charts/bso/templates/crds/imagescanjobs.yaml && rm charts/bso/templates/crds/imagescanjobs.yaml.tmp
	cp config/crd/bases/security.blacksyrius.com_notificationpolicies.yaml \
	   charts/bso/templates/crds/notificationpolicies.yaml.tmp && \
	{ printf '{{- if .Values.installCRDs }}\n'; cat charts/bso/templates/crds/notificationpolicies.yaml.tmp; printf '\n{{- end }}\n'; } \
	   > charts/bso/templates/crds/notificationpolicies.yaml && rm charts/bso/templates/crds/notificationpolicies.yaml.tmp
	cp config/crd/bases/security.blacksyrius.com_scanpolicies.yaml \
	   charts/bso/templates/crds/scanpolicies.yaml.tmp && \
	{ printf '{{- if .Values.installCRDs }}\n'; cat charts/bso/templates/crds/scanpolicies.yaml.tmp; printf '\n{{- end }}\n'; } \
	   > charts/bso/templates/crds/scanpolicies.yaml && rm charts/bso/templates/crds/scanpolicies.yaml.tmp
	cp config/crd/bases/security.blacksyrius.com_securityscanconfigs.yaml \
	   charts/bso/templates/crds/securityscanconfigs.yaml.tmp && \
	{ printf '{{- if .Values.installCRDs }}\n'; cat charts/bso/templates/crds/securityscanconfigs.yaml.tmp; printf '\n{{- end }}\n'; } \
	   > charts/bso/templates/crds/securityscanconfigs.yaml && rm charts/bso/templates/crds/securityscanconfigs.yaml.tmp

.PHONY: helm-lint
helm-lint: ## Lint the BSO Helm chart.
	helm lint charts/bso/

.PHONY: helm-template
helm-template: ## Render Helm chart templates for inspection (web.enabled=true).
	helm template bso charts/bso/ --set web.enabled=true

##@ All-in-one installer

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply --validate=false -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.20.1

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.8.0
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))
	@test -f .custom-gcl.yml && { \
		echo "Building custom golangci-lint with plugins..." && \
		$(GOLANGCI_LINT) custom --destination $(LOCALBIN) --name golangci-lint-custom && \
		mv -f $(LOCALBIN)/golangci-lint-custom $(GOLANGCI_LINT); \
	} || true

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
