VERSION ?= dev
BINARY_NAME ?= simob
GOOS ?= linux
GOARCH ?= amd64

# Default build variables
CGO_ENABLED_ENV = 0
CC_ENV =
CXX_ENV =
STATIC_LDFLAGS =

# Handle Linux-specific Zig cross-compilation
ifeq ($(GOOS),linux)
  CGO_ENABLED_ENV = 1

  ifeq ($(GOARCH),amd64)
    ZIG_TARGET = x86_64-linux-musl
  else ifeq ($(GOARCH),arm64)
    ZIG_TARGET = aarch64-linux-musl
  endif

  ifdef ZIG_TARGET
    CC_ENV = CC="zig cc -target $(ZIG_TARGET)"
    CXX_ENV = CXX="zig c++ -target $(ZIG_TARGET)"
    STATIC_LDFLAGS = -linkmode external -extldflags '-static'
  endif
endif

BUILD_ENV = GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED_ENV) $(CC_ENV) $(CXX_ENV)

.PHONY: all build-debug build-prod

all: build-debug build-prod

build-debug:
  @echo "Building debug version: $(VERSION).debug"
  @echo "Output binary name: $(BINARY_NAME)-debug"
  $(BUILD_ENV) go build \
    -gcflags="all=-N -l" \
    -ldflags "$(STATIC_LDFLAGS) -X 'agent/internal/version.Version=$(VERSION).debug'" \
    -o ../$(BINARY_NAME)-debug \
    main.go

build-prod:
  @echo "Building production version: $(VERSION)"
  @echo "Output binary name: $(BINARY_NAME)"
  $(BUILD_ENV) go build \
    -ldflags "-s -w $(STATIC_LDFLAGS) -X 'agent/internal/version.Version=$(VERSION)'" \
    -o ../$(BINARY_NAME) \
    main.go