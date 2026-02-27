BINARY_NAME := snippets
DATE_FMT=+%Y-%m-%d
VERSION := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date "$(DATE_FMT)")
BUILD_FLAGS := "-w -s -X 'main.Version=$(VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'"
BUILD_DIR := $(CURDIR)/build
INSTALL_DIR := ~/bin
GO_FILES := $(shell find . -name '*.go' -not -path "./vendor/*")
OS_ARCHES := linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64

# build these rules even if files with the same name exist.
.PHONY: build clean test install uninstall

clean:
	rm -rf $(BUILD_DIR)

build:
	@for os_arch in $(OS_ARCHES); do \
		os=$$(echo $$os_arch | cut -d'/' -f1); \
		arch=$$(echo $$os_arch | cut -d'/' -f2); \
		bin_suffix=$$(if [ "$$os" = "windows" ]; then echo ".exe"; else echo ""; fi); \
		echo "Building for $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
		go build \
		-tags=netgo \
		-ldflags=$(BUILD_FLAGS) \
		-o $(BUILD_DIR)/$$os_arch/$(BINARY_NAME)$$bin_suffix \
		$(GO_FILES);\
	done

test:
	CGO_ENABLED=0 go test -v ./...

# build and install the binary
install:
	mkdir -p $(INSTALL_DIR)
	@os_arch=$$(go env GOOS)/$$(go env GOARCH); \
		bin_suffix=$$(if [ "$$(go env GOOS)" = "windows" ]; then echo ".exe"; else echo ""; fi); \
		cp $(BUILD_DIR)/$$os_arch/$(BINARY_NAME)$$bin_suffix $(INSTALL_DIR)/$(BINARY_NAME)$$bin_suffix
	echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)"

# uninstall the binary
uninstall:
	@local_bin_name=$(INSTALL_DIR)/$(BINARY_NAME)$$(if [ "$$(go env GOOS)" = "windows" ]; then echo ".exe"; else echo ""; fi);\
	echo "rm -f $$local_bin_name";\
	rm -f $$local_bin_name
	echo "Uninstalled $(BINARY_NAME) from $(INSTALL_DIR)"
