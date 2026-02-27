.PHONY: build clean install test fmt vet

# Binary name
BINARY_NAME=ubuntu-mirror-auditor

# Build directory
BUILD_DIR=.

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

all: test build

build:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

clean:
	$(GOCLEAN)
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f *.db
	rm -rf downloads/

install: build
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

test:
	$(GOTEST) -v ./...

fmt:
	$(GOFMT) ./...

vet:
	$(GOVET) ./...

deps:
	$(GOGET) -v ./...

run-list:
	$(BUILD_DIR)/$(BINARY_NAME) list

run-help:
	$(BUILD_DIR)/$(BINARY_NAME) --help
