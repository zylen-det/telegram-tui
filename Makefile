TDLIB_PREFIX ?= $(CURDIR)/.local/tdlib
VERSION ?= dev

.PHONY: fmt test race vet verify tdlib tdlib-source build test-tdlib package

fmt:
	@set -- internal; \
	if test -d cmd; then set -- "$$@" cmd; fi; \
	files="$$(find "$$@" -type f -name '*.go')" || exit $$?; \
	if test -z "$$files"; then exit 0; fi; \
	unformatted="$$(gofmt -l $$files)"; status=$$?; \
	if test -n "$$unformatted"; then printf '%s\n' "$$unformatted"; fi; \
	if test "$$status" -ne 0; then exit "$$status"; fi; \
	test -z "$$unformatted"

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

verify: fmt test race vet

tdlib:
	TDLIB_PREFIX="$(TDLIB_PREFIX)" ./scripts/install-tdlib-prebuilt.sh

tdlib-source:
	TDLIB_PREFIX="$(TDLIB_PREFIX)" ./scripts/build-tdlib.sh

build:
	mkdir -p bin
	CGO_ENABLED=1 \
	CGO_CFLAGS="-I$(TDLIB_PREFIX)/include" \
	CGO_LDFLAGS="-Wl,-rpath,$(TDLIB_PREFIX)/lib -L$(TDLIB_PREFIX)/lib -ltdjson" \
	go build -trimpath -tags 'tdlib libtdjson' \
		-ldflags "-X github.com/zylen-det/telegram-tui/internal/buildinfo.version=$(VERSION)" \
		-o bin/telegram-tui ./cmd/telegram-tui

test-tdlib:
	CGO_ENABLED=1 \
	CGO_CFLAGS="-I$(TDLIB_PREFIX)/include" \
	CGO_LDFLAGS="-Wl,-rpath,$(TDLIB_PREFIX)/lib -L$(TDLIB_PREFIX)/lib -ltdjson" \
	go test -tags 'tdlib libtdjson' ./internal/telegram

package:
	@test "$(VERSION)" != "dev" || { printf 'VERSION is required (for example, make package VERSION=v0.1.0)\n' >&2; exit 1; }
	TDLIB_PREFIX="$(TDLIB_PREFIX)" VERSION="$(VERSION)" ./scripts/package-release.sh
