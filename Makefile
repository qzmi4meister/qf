SHELL := /bin/bash

# BPF toolchain
BPF_CC     ?= clang
BPF_ARCH   ?= $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')
BPF_CFLAGS ?= -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(BPF_ARCH)

BUF ?= buf

# Packaging / images
QF_VERSION  := $(shell sed -n 's/.*Version = "\([^"]*\)".*/\1/p' version/version.go)
PKG_ARCH    := $(shell go env GOARCH)
QF_IMAGE    ?= localhost/qf-cp
HELM_REGISTRY ?= oci://ghcr.io/YOUR_ORG/helm

.PHONY: all generate proto bpf build build-cp ui-install ui-build ui-dev bench-bpf bench-fanout bench-ingest bench-conntrack pkg-agent docker-cp helm-package test-pki clean

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

# PKI unit + token integration tests — token tests require PostgreSQL (QF_TEST_DSN).
# Run via SSH: set QF_TEST_DSN to the qf database DSN on the remote host.
test-pki:
	ssh qf 'cd /opt/qf && QF_TEST_DSN="$(QF_TEST_DSN)" go test -race -count=1 ./cp/internal/pki/ 2>&1'

# Event ingest benchmark — requires PostgreSQL; set QF_BENCH_DSN on remote host.
# Benchmarks self-skip if QF_BENCH_DSN is not set.
bench-ingest:
	ssh qf 'cd /opt/qf && go test -bench=BenchmarkIngest -benchmem -benchtime=30s -run=^$$ -count=1 ./cp/internal/ingest/ 2>&1'

# Bundle fan-out benchmark — pure Go, runs locally or on remote.
# Reports p50/p95/p99/max per-agent delivery latency at concurrency 1/50/100/200.
bench-fanout:
	go test -bench=BenchmarkFanOut_ -benchmem -benchtime=10s -run=^$$ -count=1 ./cp/internal/agentsrv/ 2>&1

# Conntrack LRU stress bench — must run on Linux with CAP_BPF (SSH to remote).
# Fills 65536-entry LRU map, measures BPF throughput at saturation + hit-rate.
bench-conntrack:
	ssh qf 'cd /opt/qf && go test -bench=BenchmarkConntrack_ -benchmem -benchtime=50000x -run=TestConntrackLRU_ -count=1 ./agent/internal/loader/ 2>&1'

# BPF datapath benchmarks — must run on Linux with CAP_BPF (SSH to remote).
# -benchtime=50000x: b.N=50000 kernel iterations per bench; stable ns/pkt reading.
# Requires CAP_BPF on remote; if EPERM, benchmarks self-skip with message.
bench-bpf:
	ssh qf 'cd /opt/qf && go test -bench=BenchmarkBPF_ -benchmem -benchtime=50000x -run=^$$ -count=3 ./agent/internal/loader/ 2>&1'

# Build multi-arch CP container image via podman (amd64 + arm64).
# Produces a local manifest list at $(QF_IMAGE):$(QF_VERSION).
# Requires: podman with QEMU binfmt support for cross-arch.
# To push: podman manifest push --all $(QF_IMAGE):$(QF_VERSION) <registry>/qf-cp:$(QF_VERSION)
docker-cp:
	podman build \
		--platform linux/amd64,linux/arm64 \
		--manifest $(QF_IMAGE):$(QF_VERSION) \
		-f deploy/Dockerfile.cp \
		.

# Package Helm chart into a .tgz in deploy/helm/.
# Requires: helm
helm-package:
	helm package deploy/helm/qf-cp/ -d deploy/helm/

# Push Helm chart to GHCR OCI registry.
# Requires: helm login ghcr.io -u <user> -p <token>
helm-push: helm-package
	helm push deploy/helm/qf-cp-$(QF_VERSION).tgz $(HELM_REGISTRY)

# Package qf-agent as .deb and .rpm via nfpm.
# Requires: nfpm (https://nfpm.goreleaser.com/), go build must run first.
pkg-agent: build
	VERSION=$(QF_VERSION) ARCH=$(PKG_ARCH) nfpm package -f deploy/packaging/nfpm.yaml -p deb -t deploy/packaging/deb/
	VERSION=$(QF_VERSION) ARCH=$(PKG_ARCH) nfpm package -f deploy/packaging/nfpm.yaml -p rpm -t deploy/packaging/rpm/

# Publish agent packages to GitHub Release v$(QF_VERSION).
# Requires: gh auth login
release-agent: pkg-agent
	gh release create v$(QF_VERSION) --title "v$(QF_VERSION)" --notes "" 2>/dev/null || true
	gh release upload v$(QF_VERSION) deploy/packaging/deb/qf-agent_$(QF_VERSION)_*.deb --clobber
	gh release upload v$(QF_VERSION) deploy/packaging/rpm/qf-agent-$(QF_VERSION).*.rpm --clobber

# Deploy agent to all hosts (or specific host: make deploy-agent target=hbfw2).
# Requires: ansible (pip install ansible)
ANSIBLE_INVENTORY ?= .agents-meta/ansible-inventory.yml
deploy-agent:
	~/venv/ansible-default/bin/ansible-playbook -i $(ANSIBLE_INVENTORY) deploy/ansible/deploy-agent.yml \
		$(if $(target),--limit $(target),)

clean:
	$(MAKE) -C agent/bpf clean
	rm -f qf-agent qf-cp
	rm -f agent/internal/loader/tc_filter_bpf*.go
	rm -f agent/internal/loader/tc_filter_bpf*.o
	rm -rf cp/internal/embeddedui/dist/assets
