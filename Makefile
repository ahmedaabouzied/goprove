# GoProve Makefile
# Handles building with version info injected via ldflags.
# Outputs to target/<os>-<arch>/ (e.g., target/darwin-arm64/goprove).

BINARY_NAME := goprove

# Detect current platform. Override with: make build GOOS=linux GOARCH=amd64
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Build output: target/<os>-<arch>/goprove
TARGET_DIR := target/$(GOOS)-$(GOARCH)

# Version info extracted from git.
# VERSION: latest git tag (e.g., v0.1.0). Empty if no tags exist.
# COMMIT: short commit hash of HEAD (e.g., abc1234).
# DATE: UTC build date in YYYY-MM-DD format.
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null)
COMMIT := $(shell git rev-parse --short HEAD)
DATE := $(shell date -u +%Y-%m-%d)

# ldflags injection targets — these set the package-level vars in pkg/version.
PKG := github.com/ahmedaabouzied/goprove/pkg/version
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

# Default target: build for the current platform with version info injected.
.PHONY: build
build:
	@mkdir -p $(TARGET_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(TARGET_DIR)/$(BINARY_NAME) ./cmd/goprove/

# Dev build: no ldflags. Version falls back to debug.ReadBuildInfo().
.PHONY: dev
dev:
	@mkdir -p $(TARGET_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(TARGET_DIR)/$(BINARY_NAME) ./cmd/goprove/

# Cross-compile for common release targets.
.PHONY: build-all
build-all:
	GOOS=darwin GOARCH=arm64 $(MAKE) build
	GOOS=darwin GOARCH=amd64 $(MAKE) build
	GOOS=linux GOARCH=amd64 $(MAKE) build
	GOOS=linux GOARCH=arm64 $(MAKE) build

# Build and install goprove to /usr/local/bin.
.PHONY: install
install: build
	sudo install -m 755 $(TARGET_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

# Run all tests.
.PHONY: test
test:
	go test ./...

# Remove all build artifacts.
.PHONY: clean
clean:
	rm -rf target/
