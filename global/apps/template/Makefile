# Hivy app build tooling.
#
#   make deps    — npm ci (once per sandbox; node_modules is cached)
#   make bundle  — vite build + go build, assemble dist/bundle/, zip dist/bundle.zip
#   make source  — zip tracked sources into dist/source.zip
#   make all     — bundle + source (what app_publish consumes)
#   make test    — go test ./... AND the web vitest suite (needs `make deps`)
#   make preview — run the app in THIS sandbox with real platform env and
#                  print the public preview URL (last line of output):
#                  make preview APP_ID=<uuid> [PORT=3000]
#
# App sandboxes are always linux/amd64, regardless of what the builder
# sandbox runs — GOOS/GOARCH default to that target below so a non-amd64 (or
# non-linux) builder can't silently produce a binary the deploy target can't
# execute. Override only if you know the deploy target changed.
#
# .NOTPARALLEL below is deliberate: `vite build` and `go build` are each
# multi-process/multi-threaded on their own, and running them at the same
# time (e.g. under `make -jN`) is what killed the Go build under memory
# pressure in the builder sandbox. `web` always finishes before `server`
# starts. GOFLAGS=-p=1 also caps Go's own package-build parallelism to keep
# its peak memory down.

# PORT here is a PREVIEW-ONLY convenience (which local port `make preview`
# listens on in THIS builder sandbox). It has no effect on production: the
# deployed app always gets PORT unset and defaults to 8080 inside
# hivycore/config.go, and the platform's sandbox alias/proxy is hard-bound to
# 8080 (sandboxes/app/hivy-app.service, internal/apps/image.go's appPort).
# Never hardcode a different port in app code to "fix" a preview conflict —
# change PORT here instead.
DIST := dist
PORT ?= 3000
GOOS ?= linux
GOARCH ?= amd64

.NOTPARALLEL:

.PHONY: all deps web server bundle source test preview clean

all: bundle source

deps:
	cd web && npm ci

web:
	cd web && npm run build

# CGO_ENABLED=0: the template and its one dependency (golang-jwt) are pure
# Go, and disabling cgo keeps cross-compilation (e.g. building for
# linux/amd64 from a non-linux or non-amd64 builder) from ever needing a C
# cross-compiler. GOFLAGS=-p=1 serializes Go's own package builds to limit
# peak memory (see .NOTPARALLEL note above).
server:
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) GOFLAGS=-p=1 go build -o $(DIST)/server .

bundle: web server
	rm -rf $(DIST)/bundle $(DIST)/bundle.zip
	mkdir -p $(DIST)/bundle
	cp $(DIST)/server $(DIST)/bundle/server
	cp -R public $(DIST)/bundle/public
	cp app.json $(DIST)/bundle/app.json
	cd $(DIST)/bundle && zip -qr ../bundle.zip .

source:
	mkdir -p $(DIST)
	rm -f $(DIST)/source.zip
	zip -qr $(DIST)/source.zip . \
		-x "web/node_modules/*" \
		-x "$(DIST)/*" \
		-x "public/*" \
		-x ".git/*" \
		-x "*.DS_Store"

test:
	go test ./...
	cd web && npm test

# Preview loop (see README "Preview loop"): rebuilds the server, fetches the
# app's real env over the authenticated side channel, restarts the preview
# supervised, waits for /healthz, and prints the preview URL as the LAST line.
# Secrets stay in the 0600 env file and process env — never in this output.
preview: server
	bash scripts/preview.sh "$(APP_ID)" "$(PORT)"

clean:
	rm -rf $(DIST) public
