SHELL = bash
PROJECT_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

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

.PHONY: tools
tools:
	@command -v mise >/dev/null 2>&1 || { \
	  echo >&2 "Error: 'mise' not found in your PATH."; \
	  echo >&2 "Quick-install: 'curl https://mise.run | sh'"; \
	  echo >&2 "Full install instructions: https://mise.jdx.dev/installing-mise.html"; \
	  exit 1; \
	}

# Only for CI compliance
.PHONY: bootstrap
bootstrap: tools # Install all dependencies
	@mise install

.PHONY: tidy
tidy:
	@echo "--> Tidy module"
	@go mod tidy

.PHONY: check
check: tools
check: ## Lint the source code
	@echo "==> Linting source code..."
	@mise x -- golangci-lint run -j 1

	@echo "==> Checking Go mod..."
	$(MAKE) tidy
	@if (git status --porcelain | grep -Eq "go\.(mod|sum)"); then \
		echo go.mod or go.sum needs updating; \
		git --no-pager diff go.mod; \
		git --no-pager diff go.sum; \
		exit 1; fi

.PHONY: gofmt
gofmt: tools
gofmt: ## Format Go code
	@mise x -- gofumpt -extra -l -w .

.PHONY: gogenerate
gogenerate: ## Generate code from Go code
	@go generate ./...

.PHONY: dev
dev: tools
dev: GOOS=$(shell go env GOOS)
dev: GOARCH=$(shell go env GOARCH)
dev: GOPATH=$(shell go env GOPATH)
dev: DEV_TARGET=dist/$(PROJECT_NAME)_$(GOOS)_*/$(PROJECT_NAME)
dev: ## Build for the current development platform
	@echo "==> Removing old development build..."
	@rm -fv $(PROJECT_ROOT)/bin/$(PROJECT_NAME)
	@rm -fv $(GOPATH)/bin/$(PROJECT_NAME)

	@echo "==> Building only for current GOOS and GOARCH..."
	mise x -- goreleaser build \
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
snapshot: tools
snapshot: ## Build snapshot packages
	@echo "==> Building snapshot packages..."
	mise x -- goreleaser release \
		$(if $(VERBOSE),--verbose) \
		--snapshot \
		--clean

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
