SHELL := /bin/bash

# BPF toolchain
BPF_CC     ?= clang
BPF_ARCH   ?= $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')
BPF_CFLAGS ?= -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(BPF_ARCH)

BUF ?= buf

.PHONY: all generate proto bpf build clean

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

clean:
	$(MAKE) -C agent/bpf clean
	rm -f qf-agent
	rm -f agent/internal/loader/tc_filter_bpf*.go
	rm -f agent/internal/loader/tc_filter_bpf*.o
