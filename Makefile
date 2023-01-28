SHELL = bash
PROJECT_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
THIS_OS := $(shell uname)

GOTESTSUM_VERSION ?= v1.8.2
GOLANGCI_LINT ?= $(shell which golangci-lint)
GOLANGCI_LINT_VERSION ?= v1.50.1
GOFUMPT ?= $(shell which gofumpt)
GOFUMPT_VERSION ?= v0.4.0
GORELEASER ?= $(shell which goreleaser)
GORELEASER_VERSION ?= v1.14.1

# Using directory as project name.
PROJECT_NAME := $(shell basename $(PROJECT_ROOT))
PROJECT_MODULE := $(shell go list -m)

GO_TEST_CMD = $(if $(shell command -v gotestsum 2>/dev/null),gotestsum --,go test)
GO_TEST_PKGS ?= "./..."

default: help

ifeq ($(CI),true)
$(info Running in a CI environment, verbose mode is disabled)
else
VERBOSE="true"
endif

# include per-user customization after all variables are defined
-include Makefile.local

# Only for CI compliance
.PHONY: bootstrap
bootstrap: deps lint-deps build-deps # Install all dependencies

.PHONY: deps
deps: ## Install build and development dependencies
	@echo "==> Updating build dependencies..."
	@go install gotest.tools/gotestsum@$(GOTESTSUM_VERSION)

.PHONY: lint-deps
lint-deps: ## Install linter dependencies
	@echo "==> Updating linter dependencies..."
	@which golangci-lint 2>/dev/null || ( curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION) && echo "Installed golangci-lint" )
	@which gofumpt 2>/dev/null || ( go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION) && echo "Installed gofumpt" )

.PHONY: build-deps
build-deps: ## Install builder dependencies
	@echo "==> Updating builder dependencies..."
	@which goreleaser 2>/dev/null || ( pushd $$(go env GOPATH)/bin && curl -sfL https://i.jpillora.com/goreleaser/goreleaser@$(GORELEASER_VERSION) | sh && echo "Installed goreleaser" )

.PHONY: tidy
tidy:
	@echo "--> Tidy module"
	@go mod tidy

.PHONY: check
check: ## Lint the source code
	@echo "==> Linting source code..."
	$(GOLANGCI_LINT) run -j 1

	@echo "==> Checking Go mod..."
	$(MAKE) tidy
	@if (git status --porcelain | grep -Eq "go\.(mod|sum)"); then \
		echo go.mod or go.sum needs updating; \
		git --no-pager diff go.mod; \
		git --no-pager diff go.sum; \
		exit 1; fi

.PHONY: gofmt
gofmt: ## Format Go code
	$(GOFUMPT) -extra -l -w .

.PHONY: gogenerate
gogenerate: ## Generate code from Go code
	@go generate ./...

.PHONY: dev
dev: GOOS=$(shell go env GOOS)
dev: GOARCH=$(shell go env GOARCH)
dev: GOPATH=$(shell go env GOPATH)
dev: DEV_TARGET=dist/$(PROJECT_NAME)_$(GOOS)_*/$(PROJECT_NAME)
dev: ## Build for the current development platform
	@echo "==> Removing old development build..."
	@rm -fv $(PROJECT_ROOT)/bin/$(PROJECT_NAME)
	@rm -fv $(GOPATH)/bin/$(PROJECT_NAME)

	@echo "==> Building only for current GOOS and GOARCH..."
	$(GORELEASER) build \
		$(if $(VERBOSE),--debug) \
		--snapshot \
		--rm-dist \
		--single-target

	@find $(PROJECT_ROOT)/dist
	@mkdir -p $(PROJECT_ROOT)/bin
	@mkdir -p $(GOPATH)/bin
	@cp -v $(PROJECT_ROOT)/$(DEV_TARGET) $(PROJECT_ROOT)/bin/
	@cp -v $(PROJECT_ROOT)/$(DEV_TARGET) $(GOPATH)/bin

.PHONY: snapshot
snapshot: ## Build snapshot packages
	@echo "==> Building snapshot packages..."
	$(GORELEASER) release \
		$(if $(VERBOSE),--debug) \
		--snapshot \
		--rm-dist

	@find $(PROJECT_ROOT)/dist

.PHONY: test
test: ## Run the test suite and/or any other tests
	@echo "==> Running test suites..."
	$(if $(ENABLE_RACE),GORACE="strip_path_prefix=$(GOPATH)/src") $(GO_TEST_CMD) \
		$(if $(ENABLE_RACE),-race) $(if $(VERBOSE),-v) \
		-cover \
		-coverprofile=coverage.out \
		-covermode=atomic \
		-timeout=15m \
		$(GO_TEST_PKGS)

.PHONY: coverage
coverage: ## Open a web browser displaying coverage
	go tool cover -html=coverage.out

.PHONY: clean
clean: ## Remove build artifacts
	@echo "==> Cleaning build artifacts..."
	@rm -fv coverage.out
	@find . -name '*.test' | xargs rm -fv
	@rm -rfv "$(PROJECT_ROOT)/bin/"
	@rm -rfv "$(PROJECT_ROOT)/dist/"
	@rm -rfv "$(PROJECT_ROOT)/$(PROJECT_NAME)"
	@rm -fv "$(GOPATH)/bin/$(PROJECT_NAME)"

HELP_FORMAT="    \033[36m%-22s\033[0m %s\n"
.PHONY: help
help: ## Display this usage information
	@echo "Valid targets:"
	@grep -E '^[^ ]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; \
			{printf $(HELP_FORMAT), $$1, $$2}'
	@echo

FORCE:
