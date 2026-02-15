SHELL = bash
PROJECT_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

# Using directory as project name.
PROJECT_NAME := $(shell basename $(PROJECT_ROOT))

# ═══════════════════════════════════════════════════════════════════════════
# This project uses mise (https://mise.jdx.dev) for project management.
#
# To get started:
#   1. Run 'make' to see available mise tasks
#   2. If mise is not installed, you'll get installation instructions
#   3. Run 'mise install' to install project dependencies
#   4. Run 'mise run <task>' to execute any task
# ═══════════════════════════════════════════════════════════════════════════

default: welcome

.PHONY: welcome
welcome: tools ## Get started - shows available mise tasks
	@echo ""
	@echo "╔═══════════════════════════════════════════════════════════"
	@echo "║ '$(PROJECT_NAME)'"
	@echo "║ Using mise for project management"
	@echo "╚═══════════════════════════════════════════════════════════"
	@echo ""
	@echo "Available mise tasks:"
	@echo ""
	@mise tasks
	@echo ""
	@echo "-> Run tasks with:  mise run <task>"
	@echo "-> Install deps:    mise install"
	@echo ""

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

.PHONY: tasks
tasks: tools ## List all available mise tasks
	@mise tasks
