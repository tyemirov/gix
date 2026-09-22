GO_TOOLCHAIN_ROOT := $(shell go env GOROOT)
GOFMT := $(GO_TOOLCHAIN_ROOT)/bin/gofmt
GO_SOURCES := $(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./.git/*" -not -path "*/.git/*")
FAST_TEST_PACKAGES := $(shell go list ./... | grep -v '/tests$$')
GO_TEST_FLAGS ?=
INTEGRATION_TEST_PARALLELISM := 4
STATICCHECK_MODULE := honnef.co/go/tools/cmd/staticcheck@master
INEFFASSIGN_MODULE := github.com/gordonklaus/ineffassign@latest
LICENSE_ROLLOUT_SCRIPT := scripts/licensing/license_rollout.py
LICENSE_ROLLOUT_MANIFEST := configs/licensing/fleet.json
LICENSE_ROLLOUT_WORKFLOW := configs/license-rollout.yaml

.PHONY: format check-format lint test test-unit test-integration test-fast test-slow test-licensing build license-rollout-plan license-rollout-apply release publish deploy ci

format:
	"$(GOFMT)" -w $(GO_SOURCES)

check-format:
	@formatted_files="$$('$(GOFMT)' -l $(GO_SOURCES))" || exit $$?; \
	if [ -n "$$formatted_files" ]; then \
		echo "Go files require formatting:"; \
		echo "$$formatted_files"; \
		exit 1; \
	fi

lint:
	go vet ./...
	go run $(STATICCHECK_MODULE) ./...
	go run $(INEFFASSIGN_MODULE) ./...

test-fast:
	go test $(GO_TEST_FLAGS) $(FAST_TEST_PACKAGES)
	$(MAKE) test-licensing

test-licensing:
	python3 -m unittest discover -s scripts/licensing -p 'test_*.py'

test-slow:
	go test -v -parallel=$(INTEGRATION_TEST_PARALLELISM) $(GO_TEST_FLAGS) ./tests

.PHONY: test-sync
test-sync:
	go test -v -parallel=$(INTEGRATION_TEST_PARALLELISM) $(GO_TEST_FLAGS) ./tests -run '^TestSync'

test-unit: test-fast

test-integration: test-slow

test: test-fast test-slow

build:
	mkdir -p bin
	go build -o bin/gix .

license-rollout-plan:
	timeout -k 350s -s SIGKILL 350s python3 "$(LICENSE_ROLLOUT_SCRIPT)" plan --manifest "$(LICENSE_ROLLOUT_MANIFEST)"

license-rollout-apply: build
	timeout -k 350s -s SIGKILL 350s python3 "$(LICENSE_ROLLOUT_SCRIPT)" apply --manifest "$(LICENSE_ROLLOUT_MANIFEST)" --workflow "$(LICENSE_ROLLOUT_WORKFLOW)" --gix bin/gix

MPRLAB_GATEWAY_EXECUTABLE ?= mprlab-gateway

.PHONY: release publish deploy

release publish deploy:
	@application_root="$$(git rev-parse --show-toplevel)"; \
	if ! command -v "$(MPRLAB_GATEWAY_EXECUTABLE)" >/dev/null 2>&1; then \
		printf 'Gateway runtime is unavailable: %s. Install a released runtime and add its command directory to PATH.\n' \
			"$(MPRLAB_GATEWAY_EXECUTABLE)" >&2; \
		exit 2; \
	fi; \
	exec "$(MPRLAB_GATEWAY_EXECUTABLE)" "app-$@" --app-root "$${application_root}"

ci: check-format lint test-fast test-slow

.PHONY: test-docs-browser
test-docs-browser:
	go test $(GO_TEST_FLAGS) ./cmd/cli -run '^TestDocumentationSharedUI' -count=1

.PHONY: test-merge-eval
test-merge-eval:
	@test -n "$(GIX_MERGE_EVAL_CONFIG)" || (echo "Set GIX_MERGE_EVAL_CONFIG to a provider configuration file."; exit 1)
	go test -v ./tests -run '^TestSyncResolutionPlanProviderEvaluation$$' -count=1 -timeout=60m

.PHONY: test-ledger-e2e
test-ledger-e2e:
	@test -n "$(GIX_MERGE_EVAL_CONFIG)" || (echo "Set GIX_MERGE_EVAL_CONFIG to a provider configuration file."; exit 1)
	go test -v ./tests -run '^TestSyncLedgerStashProviderEvaluation$$' -count=1 -timeout=6m
