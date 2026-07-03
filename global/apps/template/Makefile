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
# GOOS/GOARCH pass through for cross-builds; the deploy target is linux/amd64
# (builder sandboxes are already linux, so the default native build is right
# there).

DIST := dist
PORT ?= 3000

.PHONY: all deps web server bundle source test preview clean

all: bundle source

deps:
	cd web && npm ci

web:
	cd web && npm run build

server:
	mkdir -p $(DIST)
	go build -o $(DIST)/server .

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
