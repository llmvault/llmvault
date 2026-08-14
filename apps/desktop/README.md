# Hivy desktop

The desktop app is a small Tauri shell around the production web frontend. It
doesn't copy React components, hooks, stores, API types, or session code. During
development it opens the local web service. Release builds read their web origin
from `HIVY_DESKTOP_CLOUD_URL` at build time.

Tauri starts one Rust runtime on a free loopback port. The runtime keeps each
agent's proxy and MCP configuration in memory, stores its loopback bearer in the
operating system credential manager, and uses SQLite only for local session
history and runtime events. Agent turns execute on the user's machine. Hivy's
API still handles login, the model proxy, MCP, session metadata, and cloud event
backup. Desktop sessions do not accrue hosted-sandbox compute charges.

## Local development

Start the cloud stack from the repository root:

```sh
make dev
```

In another terminal, start the native app:

```sh
make desktop-dev
```

Run the real login, model, local-runtime, and cloud-backup check with the same
temporary dev bearer passed to the app:

```sh
HIVY_DESKTOP_RUNTIME_SECRET=desktop-e2e-local-secret \
  HIVY_DESKTOP_RUNTIME_URL=http://127.0.0.1:37080 make desktop-dev
HIVY_DESKTOP_RUNTIME_SECRET=desktop-e2e-local-secret make desktop-e2e
```

The script uses the seeded `dev@hivy.local` account and expects `make dev` or
`make up` to expose the API on port 8080.

## Tests and packaging

```sh
make desktop-test
make desktop-build
```

Set `HIVY_DESKTOP_CLOUD_URL` to the production HTTPS web origin in the release
job. The local build command defaults it to `http://localhost:30112` so the
resulting app can be checked against `make dev`.

`desktop-build` compiles the runtime in release mode, places it in the Tauri
resource bundle, and produces the native installers under
`apps/desktop/src-tauri/target/release/bundle`.

## Security boundary

Remote web content can register an agent configuration, deliver a message for
that agent, and read the local repository tree, file content, and diffs used by
the existing Files and Review panels. It can't call the runtime's shell-control
endpoint, read its configuration, or retrieve the bearer token. The local HTTP
server binds to loopback and checks a 64-character credential stored in
Keychain, Credential Manager, or Secret Service through the Rust `keyring`
crate. Repository reads also pass through the native bridge, so browser code
never receives that credential.

Desktop agent changes wait for the current turn to finish. That keeps one
agent's MCP token and outbound channel configuration out of another agent's
turn while still running every local agent in one runtime process.
