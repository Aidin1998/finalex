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
ACCOUNTS_BINARY  := cex-accounts

# Directories
BUILD_DIR	?= ./build
IDENTITIES_DIR 	:= ./cmd/identities
ACCOUNTS_DIR	:= ./cmd/accounts

.PHONY: all build clean test fmt lint run deps build-identities build-accounts diff diff-identities

all: test build

build: build-identities build-accounts

build-identities:
	$(GOBUILD) -o $(BUILD_DIR)/$(IDENTITIES_BINARY) -v $(IDENTITIES_DIR)

build-accounts:
	$(GOBUILD) -o $(BUILD_DIR)/$(ACCOUNTS_BINARY) -v $(ACCOUNTS_DIR)

clean:
	$(GOCLEAN)
	rm -f $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

fmt:
	$(GOFMT) -l -w .

lint:
	$(LINTER) run --config $(LINTCONFIG)

deps:
	$(GOGET) -v ./...

# Cross compilation
build-linux-identities:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(IDENTITIES_BINARY)_linux -v $(IDENTITIES_DIR)

build-linux-accounts:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(ACCOUNTS_BINARY)_linux -v $(ACCOUNTS_DIR)

build-linux: build-linux-identities build-linux-accounts

