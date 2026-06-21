#!/usr/bin/env bash
set -euo pipefail
export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"

IMAGE="${HIVY_SANDBOXES_RUNTIME_IMAGE:-ghcr.io/usehivy/hivy-sandboxes-runtime-developers:runtime}"
DOCKER_BIN="${DOCKER_BIN:-$(command -v docker)}"
PLATFORM="${HIVY_SANDBOXES_RUNTIME_PLATFORM:-$("$DOCKER_BIN" image inspect -f '{{.Os}}/{{.Architecture}}' "$IMAGE" 2>/dev/null || true)}"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cat >"$TMP_DIR/smoke.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

wait_for_http() {
  local url="$1"
  local pid="$2"
  local log_path="$3"

  for _ in $(seq 1 120); do
    if curl -fsS "$url" >/tmp/smoke-response.html 2>/dev/null; then
      return 0
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      cat "$log_path" >&2 || true
      return 1
    fi
    sleep 0.5
  done

  cat "$log_path" >&2 || true
  return 1
}

stop_server() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

echo "== baseline versions =="
ruby -v
gem -v
bundle -v
php -v | head -n 1
composer --version
node -v
pnpm -v
yarn -v
docker --version
dockerd --version
docker compose version
command -v hivy-start-docker

if docker info >/dev/null 2>&1; then
  echo "docker must not be running before an explicit start" >&2
  exit 1
fi

if command -v rails >/dev/null; then
  echo "rails should not be globally installed in the image" >&2
  exit 1
fi

workdir="$(mktemp -d)"
cd "$workdir"

echo "== rails app smoke =="
gem install --no-document rails >/tmp/rails-gem-install.log
rails new rails_smoke \
  --skip-git \
  --skip-javascript \
  --skip-hotwire \
  --skip-action-mailbox \
  --skip-action-text \
  --skip-active-storage \
  --skip-action-cable \
  --skip-test \
  --skip-system-test \
  --skip-bootsnap
cd rails_smoke
bin/rails server -b 127.0.0.1 -p 3010 >/tmp/rails-server.log 2>&1 &
rails_pid="$!"
wait_for_http "http://127.0.0.1:3010" "$rails_pid" /tmp/rails-server.log
stop_server "$rails_pid"
cd "$workdir"
echo "rails app started"

echo "== laravel app smoke =="
composer create-project laravel/laravel laravel_smoke --no-interaction --prefer-dist >/tmp/laravel-create.log
cd laravel_smoke
php artisan serve --host=127.0.0.1 --port=8010 >/tmp/laravel-server.log 2>&1 &
laravel_pid="$!"
wait_for_http "http://127.0.0.1:8010" "$laravel_pid" /tmp/laravel-server.log
stop_server "$laravel_pid"
echo "laravel app started"

echo "developers image smoke passed"
SH

run_args=()
if [[ -n "$PLATFORM" ]]; then
  run_args+=(--platform "$PLATFORM")
fi

"$DOCKER_BIN" run --rm \
  "${run_args[@]+"${run_args[@]}"}" \
  --entrypoint /bin/bash \
  -v "$TMP_DIR/smoke.sh:/tmp/smoke.sh:ro" \
  "$IMAGE" \
  /tmp/smoke.sh
