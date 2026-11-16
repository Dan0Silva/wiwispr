.PHONY: all build test install setup shortcut clean

BINARY_NAME := wiwispr
INSTALL_DIR := $(HOME)/.local/bin

all: build

build:
	@mkdir -p bin
	go build -ldflags="-s -w" -o bin/$(BINARY_NAME) ./cmd/wiwispr

test:
	go test -v ./...

install: build
	@mkdir -p $(INSTALL_DIR)
	cp -f bin/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✓ Installed to $(INSTALL_DIR)/$(BINARY_NAME)"

setup:
	chmod +x scripts/setup.sh
	./scripts/setup.sh

shortcut:
	chmod +x scripts/register-shortcut.sh
	./scripts/register-shortcut.sh

clean:
	rm -rf bin/
