SHELL := /bin/bash

# BPF toolchain
BPF_CC     ?= clang
BPF_ARCH   ?= $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')
BPF_CFLAGS ?= -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(BPF_ARCH)

BUF ?= buf

.PHONY: all generate proto bpf build build-cp ui-install ui-build ui-dev bench-bpf clean

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

# BPF datapath benchmarks — must run on Linux with CAP_BPF (SSH to remote).
# -benchtime=50000x: b.N=50000 kernel iterations per bench; stable ns/pkt reading.
# Requires CAP_BPF on remote; if EPERM, benchmarks self-skip with message.
bench-bpf:
	ssh qf 'cd /opt/qf && go test -bench=BenchmarkBPF_ -benchmem -benchtime=50000x -run=^$$ -count=3 ./agent/internal/loader/ 2>&1'

clean:
	$(MAKE) -C agent/bpf clean
	rm -f qf-agent qf-cp
	rm -f agent/internal/loader/tc_filter_bpf*.go
	rm -f agent/internal/loader/tc_filter_bpf*.o
	rm -rf cp/internal/embeddedui/dist/assets
