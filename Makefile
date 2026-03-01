MODULE = github.com/digitalocean/karpenter-provider-digitalocean
BINARY = karpenter-do
IMG ?= digitalocean/karpenter-provider-do:latest

# Go tooling
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen
ENVTEST ?= go run sigs.k8s.io/controller-runtime/tools/setup-envtest

# Directories
TOOLS_DIR := hack
API_DIR := pkg/apis/v1alpha1
CRD_DIR := charts/karpenter-do/templates/crds

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Build

.PHONY: build
build: ## Build controller binary
	go build -o bin/$(BINARY) ./cmd/controller/

.PHONY: run
run: generate ## Run controller from host
	go run ./cmd/controller/

.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push $(IMG)

##@ Code Generation

.PHONY: generate
generate: ## Generate deepcopy, CRDs, and other code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/..."
	$(CONTROLLER_GEN) crd paths="./pkg/apis/..." output:crd:artifacts:config=$(CRD_DIR)

.PHONY: manifests
manifests: ## Generate CRD manifests
	$(CONTROLLER_GEN) crd paths="./pkg/apis/..." output:crd:artifacts:config=$(CRD_DIR)

##@ Testing

.PHONY: test
test: ## Run unit tests
	go test ./pkg/... -coverprofile cover.out

.PHONY: test-integration
test-integration: ## Run integration tests
	go test ./test/suites/integration/... -v -count=1

.PHONY: test-e2e
test-e2e: ## Run e2e tests
	go test ./test/suites/e2e/... -v -count=1 -timeout 30m

##@ Linting

.PHONY: lint
lint: ## Run linters
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Run go fmt
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

##@ Development

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: verify
verify: generate fmt vet ## Verify generated files are up to date
	@if [ -n "$$(git diff --name-only)" ]; then \
		echo "Generated files are out of date. Run 'make generate' and commit the changes."; \
		git diff --name-only; \
		exit 1; \
	fi

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into cluster
	kubectl apply -f $(CRD_DIR)/

.PHONY: uninstall
uninstall: ## Uninstall CRDs from cluster
	kubectl delete -f $(CRD_DIR)/

.PHONY: helm-install
helm-install: ## Install Helm chart
	helm upgrade --install karpenter-do charts/karpenter-do/ \
		--namespace kube-system \
		--create-namespace

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm chart
	helm uninstall karpenter-do -n kube-system
