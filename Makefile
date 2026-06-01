SHELL := /bin/bash

# BPF toolchain
BPF_CC     ?= clang
BPF_ARCH   ?= $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')
BPF_CFLAGS ?= -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(BPF_ARCH)

BUF ?= buf

.PHONY: all generate proto bpf build build-cp ui-install ui-build ui-dev bench-bpf bench-fanout bench-ingest clean

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

# Event ingest benchmark — requires PostgreSQL; set QF_BENCH_DSN on remote host.
# Benchmarks self-skip if QF_BENCH_DSN is not set.
bench-ingest:
	ssh qf 'cd /opt/qf && go test -bench=BenchmarkIngest -benchmem -benchtime=30s -run=^$$ -count=1 ./cp/internal/ingest/ 2>&1'

# Bundle fan-out benchmark — pure Go, runs locally or on remote.
# Reports p50/p95/p99/max per-agent delivery latency at concurrency 1/50/100/200.
bench-fanout:
	go test -bench=BenchmarkFanOut_ -benchmem -benchtime=10s -run=^$$ -count=1 ./cp/internal/agentsrv/ 2>&1

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
