OUTPUT := .output
CLANG  ?= clang
ARCH   ?= $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')

# Kernel headers — override with KERN_HEADERS if not in default paths.
KERN_HEADERS ?= /usr/include

CFLAGS := \
	-g \
	-O2 \
	-Wall \
	-target bpf \
	-D__TARGET_ARCH_$(ARCH) \
	-I$(KERN_HEADERS) \
	-I/usr/include/$(shell uname -m)-linux-gnu

.PHONY: all clean

all: $(OUTPUT)/tc_filter.bpf.o

$(OUTPUT):
	mkdir -p $@

$(OUTPUT)/tc_filter.bpf.o: tc_filter.c common.h maps.h | $(OUTPUT)
	$(CLANG) $(CFLAGS) -c $< -o $@

clean:
	rm -rf $(OUTPUT)
