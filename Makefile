# Go parameters
GOCMD      ?= go
GOBUILD    := $(GOCMD) build
GOCLEAN    := $(GOCMD) clean
GOTEST     := $(GOCMD) test
GOGET      := $(GOCMD) get
GOFMT      := gofumpt
LINTER     := golangci-lint
LINTCONFIG ?= .golangci.yml


# Binary names
IDENTITIES_BINARY := cex-identities
BOOKKEEPER_BINARY := cex-bookkeeper

# Directories
BUILD_DIR ?= ./build

.PHONY: all build clean test fmt lint run deps build-identities build-bookkeeper diff diff-identities

all: test build

build: build-identities build-bookkeeper

build-identities:
	$(GOBUILD) -C ./services/identities -o ../../build/$(IDENTITIES_BINARY) -v ./cmd/identities

build-bookkeeper:
	$(GOBUILD) -C ./services/bookkeeper -o ../../build/$(BOOKKEEPER_BINARY) -v ./cmd/bookkeeper

clean:
	$(GOCLEAN)
	rm -f $(BUILD_DIR)

test:
	$(GOTEST) -v ./test/...

fmt:
	$(GOFMT) -l -w .

lint:
	$(LINTER) run --config $(LINTCONFIG)

deps:
	$(GOGET) -v ./...

# Cross compilation
build-linux-identities:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -C ./services/identities -o ../../build/$(IDENTITIES_BINARY)_linux -v ./cmd/identities
                                                                                                                             
build-linux-bookkeeper:                                                                                                      
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -C ./services/bookkeeper -o ../../build/$(BOOKKEEPER_BINARY)_linux -v ./cmd/bookkeeper

build-linux: build-linux-identities build-linux-accounts

