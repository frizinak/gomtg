
#debug: dist/
#go build -o build ./cmd/gomtg/^C& go run ./cmd/gomtg/ -i imv -ir '/bin/sh -c "imv-msg {pid} close all; imv-msg {pid} open {fn}"'  -n -c 'low:0:8' -iav  -np 2>err

#SRC := $(shell find . -type f -name '*.go') go.mod
TPL := '{{ $$root := .Dir }}{{ range .GoFiles }}{{ printf "%s/%s\n" $$root . }}{{ end }}'
DEPS = $(shell go list -f '{{ join .Deps "\n" }}' ./cmd/gomtg)
DEP_FILES = $(shell go list -f $(TPL) ./cmd/gomtg $(DEPS))

BINS := dist/gomtg-linux-amd64 dist/gomtg-darwin-amd64 dist/gomtg-linux-arm64 dist/gomtg-windows-amd64.exe

PCLIENTS:= $(patsubst dist/%, public/clients/%, $(CLIENTS))
PCLIENTS_GZ := $(foreach f, $(PCLIENTS), $(f).gz)
NATIVE := dist/gomtg-$(shell go env GOOS)-$(shell go env GOARCH)
NATIVE_DEBUG := dist/debug-gomtg

VERSION=$(shell git describe)
LDFLAGS=-s -w -X main.GitVersion=$(VERSION)

.PHONY: all
all: $(BINS)

.PHONY: install
install: $(NATIVE)
	cp -f "$(NATIVE)" "$$GOBIN/gomtg"

dist/gomtg-%: $(DEP_FILES) | dist
	GOOS=$$(echo $* | cut -d- -f1) \
		 GOARCH=$$(echo $* | cut -d- -f2 | cut -d. -f1) \
		 go build -o "$@" -trimpath -ldflags "$(LDFLAGS)" ./cmd/gomtg

$(NATIVE_DEBUG): $(DEP_FILES) | dist
	go build -tags pprof -o "$@" -trimpath -ldflags "$(LDFLAGS)" ./cmd/gomtg

dist:
	@- mkdir "$@" 2>/dev/null

.PHONY: clean
clean:
	rm -rf dist

debug: $(NATIVE_DEBUG)
	$(NATIVE_DEBUG) -i imv \
		-ir '/bin/sh -c "imv-msg {pid} close all; imv-msg {pid} open {fn}"'\
		-n \
		-c 'low:0:8' \
		-iav 9 \
		-np 2>err
