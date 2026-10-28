# Thin interface over scripts/: the same commands run on Linux, macOS and Windows,
# where `make` may not be installed at all but bash (Git Bash) is.
# Every recipe calls bash explicitly so the shell does not depend on the platform.

GO ?= go

.PHONY: help build test check fmt tools golden live clean

help: ## List the available targets
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-8s %s\n", $$1, $$2}'

build: ## Build bin/hipoteca with release ldflags
	@bash scripts/build.sh

test: ## Run the test suite
	@$(GO) test ./...

check: ## gofmt, go mod tidy, vet, staticcheck, govulncheck and tests
	@bash scripts/check.sh

fmt: ## Format the tree
	@$(GO) fmt ./...

tools: ## Install the analysis tools into ./.bin
	@bash scripts/tools.sh

golden: ## Regenerate golden files (implemented with the output contract, CORE-01)
	@UPDATE_GOLDEN=1 $(GO) test ./...

live: ## Run the tests that call the real Banco de España API
	@HIPOTECA_LIVE=1 $(GO) test -tags live ./...

clean: ## Remove build output
	@rm -rf bin dist
