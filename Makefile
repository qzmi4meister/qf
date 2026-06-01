SHELL := /bin/bash

# BPF toolchain
BPF_CC     ?= clang
BPF_ARCH   ?= $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')
BPF_CFLAGS ?= -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(BPF_ARCH)

BUF ?= buf

.PHONY: all generate proto bpf build build-cp ui-install ui-build ui-dev clean

all: generate build

# Run bpf2go codegen in loader package (requires Linux + clang).
generate:
	BPF_CFLAGS="$(BPF_CFLAGS)" go generate ./agent/internal/loader/

# Generate proto stubs (buf generate; outputs committed to repo).
proto:
	$(BUF) generate

# Compile BPF object directly (bypass bpf2go; for iterating on C code).
bpf:
	$(MAKE) -C agent/bpf

# Build Go binaries. Depends on generated files being present.
build:
	go build ./agent/cmd/qf-agent/

# Build CP binary (after ui-build).
build-cp:
	go build ./cp/cmd/qf-cp/

# Install frontend npm deps.
ui-install:
	cd ui && npm install --legacy-peer-deps

# Build frontend and embed into cp/internal/embeddedui/dist/.
ui-build: ui-install
	cd ui && npm run build

# Start Vite dev server (proxies API to localhost:8080).
ui-dev:
	cd ui && npm run dev

clean:
	$(MAKE) -C agent/bpf clean
	rm -f qf-agent qf-cp
	rm -f agent/internal/loader/tc_filter_bpf*.go
	rm -f agent/internal/loader/tc_filter_bpf*.o
	rm -rf cp/internal/embeddedui/dist/assets
