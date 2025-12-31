GO_SOURCES := $(shell git ls-files '*.go')
FAST_TEST_PACKAGES := $(shell go list ./... | grep -v '/tests$$')
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
RELEASE_DIRECTORY := dist
RELEASE_BINARY_NAME := gix
STATICCHECK_MODULE := honnef.co/go/tools/cmd/staticcheck@master
INEFFASSIGN_MODULE := github.com/gordonklaus/ineffassign@latest

.PHONY: format check-format lint test test-unit test-integration test-fast test-slow build release ci

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
	rm -rf $(RELEASE_DIRECTORY)
	mkdir -p $(RELEASE_DIRECTORY)
	for target in $(RELEASE_TARGETS); do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		output_path=$(RELEASE_DIRECTORY)/$(RELEASE_BINARY_NAME)-$$os-$$arch; \
		echo "Building $$output_path"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -o $$output_path .; \
	done

ci: check-format lint test-fast test-slow
