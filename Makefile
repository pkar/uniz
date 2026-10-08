GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)
ifdef PREFIX
BINDIR ?= $(PREFIX)/bin
else
BINDIR ?= $(HOME)/.local/bin
endif

.DEFAULT_GOAL := help
.PHONY: help build install uninstall clean fmt test vet check
help:
	@printf '%s\n' \
		'Build and install' \
		'  build    build bin/uniz' \
		'  install  install into ~/.local/bin (override BINDIR or PREFIX)' \
		'  uninstall remove the installed binary' \
		'  clean    remove bin/' \
		'' \
		'Checks' \
		'  fmt      format Go sources' \
		'  test     run tests' \
		'  vet      run go vet' \
		'  check    check formatting, vet, and test' \
		'' \
		'Variables: GO, VERSION, LDFLAGS, BINDIR, PREFIX, DESTDIR'

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/uniz ./cmd/uniz

install: build
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 0755 bin/uniz "$(DESTDIR)$(BINDIR)/uniz"

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/uniz"

clean:
	rm -rf bin

fmt:
	gofmt -w .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

check:
	test -z "$$(gofmt -l .)"
	$(GO) vet ./...
	$(GO) test ./...
