GO_SOURCES := $(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./.git/*" -not -path "*/.git/*")
FAST_TEST_PACKAGES := $(shell go list ./... | grep -v '/tests$$')
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
RELEASE_BINARY_NAME := gix
RELEASE_ARTIFACT_NAMES := gix_linux_amd64 gix_linux_arm64 gix_darwin_amd64 gix_darwin_arm64 gix_windows_amd64.exe
RELEASE_ARGS ?=
RELEASE_HELPER ?=
PUBLISH_RELEASE_ARGS ?=
RELEASE_ARTIFACT_TARGETS ?= release-artifacts pages-artifact
RELEASE_TOOL_DIR := $(CURDIR)/scripts/release
PAGES_URL ?= https://gix.mprlab.com/
PAGES_BRANCH ?= gh-pages
PAGES_VERSION ?=
PAGES_DEPLOY_ARGS ?=
STATICCHECK_MODULE := honnef.co/go/tools/cmd/staticcheck@master
INEFFASSIGN_MODULE := github.com/gordonklaus/ineffassign@latest

.PHONY: format check-format lint test test-unit test-integration test-fast test-slow build release release-artifacts pages-artifact publish-release publish deploy pages-deploy ci

format:
	gofmt -w $(GO_SOURCES)

check-format:
	@formatted_files="$$(gofmt -l $(GO_SOURCES))"; \
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
	go test $(FAST_TEST_PACKAGES)

test-slow:
	go test ./tests

test-unit: test-fast

test-integration: test-slow

test: test-fast test-slow

build:
	mkdir -p bin
	go build -o bin/gix .

release:
	@RELEASE_HELPER="$(RELEASE_HELPER)" RELEASE_ARTIFACT_TARGETS="$(RELEASE_ARTIFACT_TARGETS)" "$(RELEASE_TOOL_DIR)/prepare_release.sh" $(RELEASE_ARGS)

release-artifacts:
	@test -n "$(RELEASE_ARTIFACT_DIR)" || { echo "error: RELEASE_ARTIFACT_DIR is required" >&2; exit 1; }
	@set -eu; \
	asset_dir="$(RELEASE_ARTIFACT_DIR)/payloads/release-assets"; \
	rm -rf "$$asset_dir/bin"; \
	mkdir -p "$$asset_dir/bin"; \
	for target in $(RELEASE_TARGETS); do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		extension=""; \
		if [ "$$os" = "windows" ]; then extension=".exe"; fi; \
		output_path="$$asset_dir/bin/$(RELEASE_BINARY_NAME)_$${os}_$${arch}$${extension}"; \
		echo "Building $$output_path"; \
		if ! CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags="-s -w" -o "$$output_path" .; then \
			echo "error: failed to build release artifact for $$target" >&2; \
			exit 1; \
		fi; \
		test -f "$$output_path" || { echo "error: missing release artifact: $${output_path##*/}" >&2; exit 1; }; \
	done; \
	expected_count=0; \
	for artifact_name in $(RELEASE_ARTIFACT_NAMES); do \
		test -f "$$asset_dir/bin/$$artifact_name" || { echo "error: missing release artifact: $$artifact_name" >&2; exit 1; }; \
		expected_count=$$((expected_count + 1)); \
	done; \
	actual_count=0; \
	for artifact_path in "$$asset_dir/bin"/*; do \
		test -f "$$artifact_path" || { echo "error: release artifact set is empty" >&2; exit 1; }; \
		artifact_name=$${artifact_path##*/}; \
		case " $(RELEASE_ARTIFACT_NAMES) " in *" $$artifact_name "*) ;; *) echo "error: unexpected release artifact: $$artifact_name" >&2; exit 1 ;; esac; \
		actual_count=$$((actual_count + 1)); \
	done; \
	test "$$actual_count" -eq "$$expected_count" || { echo "error: release artifact count mismatch: expected $$expected_count, found $$actual_count" >&2; exit 1; }; \
	(cd "$$asset_dir/bin" && shasum -a 256 $(RELEASE_ARTIFACT_NAMES) > checksums.txt)

pages-artifact:
	@"$(RELEASE_TOOL_DIR)/prepare_pages_artifact.sh" --source docs --domain gix.mprlab.com --exclude GX-412-refactor-plan.md --exclude policy_refactor_plan.md --exclude readme_config_test.go

publish-release:
	@RELEASE_HELPER="$(RELEASE_HELPER)" "$(RELEASE_TOOL_DIR)/publish_release.sh" $(PUBLISH_RELEASE_ARGS)

publish: publish-release

deploy: pages-deploy

pages-deploy:
	@"$(RELEASE_TOOL_DIR)/deploy_pages_artifact.sh" --branch "$(PAGES_BRANCH)" --url "$(PAGES_URL)" $(if $(PAGES_VERSION),--version "$(PAGES_VERSION)") $(PAGES_DEPLOY_ARGS)

ci: check-format lint test-fast test-slow
