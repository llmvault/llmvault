// Static sample data for the /w design exploration.
// Everything here is fake and only exists to fill the mockup.

export interface SidebarChat {
  title: string
  meta: string
  active?: boolean
  // Agent (specialist) the session was created with; defaults to the
  // software engineer when omitted. Fixed for the life of the session.
  agentId?: string
}

export interface SidebarProject {
  name: string
  chats: SidebarChat[]
  moreChats?: SidebarChat[]
  noChats?: boolean
}

export const projects: SidebarProject[] = [
  {
    name: "constructor-io",
    chats: [
      { title: "Review unresolved PR comments", meta: "⌘1" },
      { title: "Locate SEA-1845", meta: "⌘2" },
      { title: "GIVE ME A COMPLETE BREAKDOWN of…", meta: "⌘3", agentId: "research-analyst" },
      { title: "Review PR 11181 changes", meta: "⌘4" },
      { title: "Investigate infinite v2 item calls", meta: "⌘5" },
    ],
    moreChats: [
      { title: "Debug recommendations pod restarts", meta: "4d" },
      { title: "Audit search latency dashboards", meta: "5d", agentId: "data-analyst" },
      { title: "Summarize Q2 indexing incidents", meta: "1w", agentId: "research-analyst" },
    ],
  },
  {
    name: "usehivy.com",
    chats: [
      { title: "Add Glitchtip support", meta: "3h", active: true },
      { title: "Check failing CI checks", meta: "10h" },
      { title: "Add admin integrations page", meta: "11h" },
      { title: "Add web session gateway", meta: "1d" },
      { title: "Wire up slash hero auth", meta: "1d" },
    ],
    moreChats: [
      { title: "Set up Railway preview deploys", meta: "2d" },
      { title: "Investigate Nango webhook retries", meta: "3d" },
    ],
  },
  {
    name: "ops.playfull.com",
    chats: [{ title: "Make Steam CPE chat dynamic", meta: "1d" }],
  },
  {
    name: "earnalliance.com",
    chats: [{ title: "Check top 5 steam games", meta: "1d", agentId: "research-analyst" }],
  },
  {
    name: "whatsapp.usehivy.com",
    chats: [{ title: "can you study ../usehivy.com and summ…", meta: "4d" }],
  },
  {
    name: "shiptrial.com",
    chats: [{ title: "Add codebase analysis flow", meta: "6d" }],
  },
  { name: "clickwise", chats: [] },
  { name: "hasura-ea", chats: [] },
  { name: "autocomplete", chats: [] },
  { name: "facelessnaija.com", chats: [], noChats: true },
  { name: "hakaree.com", chats: [] },
  { name: "employee.hiveloop.com", chats: [] },
]

export interface MediaAttachment {
  id: string
  filename: string
  kind: "image" | "video"
  url: string
  poster?: string
  duration?: string
}

// People with access to a session. Multiplayer is "many subscribers of one
// session log" — these drive presence, message attribution, and turn locks.
export interface Collaborator {
  id: string
  name: string
  initials: string
  color: string
  you?: boolean
}

export const collaborators: Collaborator[] = [
  { id: "you", name: "You", initials: "Y", color: "#2563eb", you: true },
  { id: "marcus", name: "Marcus Lee", initials: "M", color: "#16a34a" },
  { id: "sarah", name: "Sarah Kim", initials: "S", color: "#db2777" },
  { id: "diego", name: "Diego Alvarez", initials: "D", color: "#9333ea" },
]

// Who is currently viewing the session (for the header presence stack).
export const presentCollaborators = collaborators

export function collaboratorById(id: string): Collaborator {
  return collaborators.find((person) => person.id === id) ?? collaborators[0]
}

// Conversation blocks. Inline code is marked with backticks and split at
// render time.
export type ConversationBlock =
  | { type: "assistant"; text: string }
  | {
      type: "activity"
      label: string
      detail?: { prefix: string; file: string; adds: number; dels: number }
    }
  | {
      type: "user"
      text: string
      link?: string
      attachments?: MediaAttachment[]
      author?: Collaborator
    }
  | { type: "attachments"; items: MediaAttachment[] }
  | { type: "system"; text: string }
  | { type: "queued"; author: Collaborator; text: string }
  | { type: "worked"; duration: string; steps: string[] }
  | { type: "working"; duration?: string; by?: Collaborator }
  | { type: "tool"; label: string; running?: boolean }
  | { type: "thinking"; label?: string }
  | {
      type: "edits"
      count: number
      adds: number
      dels: number
      files: { path: string; adds: number; dels: number }[]
      moreFiles?: { path: string; adds: number; dels: number }[]
    }
  | { type: "actions" }

export const conversation: ConversationBlock[] = [
  {
    type: "assistant",
    text: "The proxy tests only prove that Hivy forwards `/api/0/*` to Nango's `/proxy/...` endpoint; they do not prove Nango uses `connection_config.baseUrl` or the real GlitchTip token correctly. I'm checking the Nango client and prod connection metadata next.",
  },
  {
    type: "assistant",
    text: "To prove the proxy path, I need the sandbox runtime bearer token, because `/internal/glitchtip-proxy/{employeeID}` does not accept the GlitchTip API key directly. I'm going to retrieve and decrypt only the existing runtime secret locally, without printing it, then call the prod proxy with that bearer.",
  },
  {
    type: "assistant",
    text: "I found the runtime secret encryption helper. I'm using Railway's DB shell to write the encrypted secret to a local temp file with no terminal output, then I'll decrypt it using the production encryption key from Railway env.",
  },
  {
    type: "assistant",
    text: "The encrypted runtime secret is in a temp file. I'm creating a tiny local helper that decrypts it and writes the plaintext to another temp file without printing it.",
  },
  {
    type: "activity",
    label: "Created 1 file",
    detail: {
      prefix: "Created",
      file: "decrypt_runtime_secret.go",
      adds: 40,
      dels: 0,
    },
  },
  {
    type: "assistant",
    text: "The helper is in `/tmp` and only writes the decrypted secret to a file. I'm running it with Railway-provided env, then I'll call the actual production proxy endpoint.",
  },
  {
    type: "assistant",
    text: "`go run` cannot import this repo's `internal` package from `/tmp`. I'm switching the helper to a standalone AES-GCM decryptor using the same encryption format, so it can run outside the repo package tree.",
  },
  {
    type: "activity",
    label: "Edited 1 file",
    detail: {
      prefix: "Edited",
      file: "decrypt_runtime_secret.go",
      adds: 12,
      dels: 9,
    },
  },
  {
    type: "assistant",
    text: "The helper is now standalone. I'm rerunning decryption and then testing the real prod Hivy proxy with the same skill paths.",
  },
  {
    type: "assistant",
    text: "The actual prod Hivy proxy still fails every documented skill path with upstream 404. Raw GlitchTip works; Hivy proxy does not. So I cannot honestly confirm “flawless once pushed” with the current proxy wiring. I'm going to patch the GlitchTip proxy to behave like Bugsink: read the stored base URL and API key, then forward directly to `{baseUrl}{path}`.",
  },
  {
    type: "assistant",
    text: "The active prod GlitchTip connection has `connection_config.baseUrl=https://glitch.usehivy.com` and credential keys `type`, `apiKey` in metadata. That is enough to bypass Nango for GlitchTip and forward directly with `Authorization: Bearer <apiKey>`.",
  },
  { type: "system", text: "Marcus Lee joined the session" },
  {
    type: "user",
    text: "please fix, commit, push",
    link: "https://github.com/usehivy/hivy/actions/runs/27352681066/job/80818504274",
    author: collaborators[1],
    attachments: [
      {
        id: "att-ci-failure",
        filename: "workspace-shell.png",
        kind: "image",
        url: "/samples/workspace-shell.png",
      },
    ],
  },
  {
    type: "worked",
    duration: "5m 57s",
    steps: [
      "Read the failing GitHub Actions job logs",
      "Reproduced the file-length failure locally with ./scripts/check-go-file-length.sh",
      "Split path/method helpers into provider_raw_proxy_paths.go",
      "Ran go test ./internal/handler and pnpm -C apps/web typecheck",
      "Committed 8295449d and pushed to main",
    ],
  },
  {
    type: "assistant",
    text: "Fixed and pushed to `main`. Root cause was the CI file-length cap: `internal/handler/provider_raw_proxy.go` had grown to 306 lines. I split the path/method helpers into `provider_raw_proxy_paths.go`, bringing the original file down to 281 lines.",
  },
  {
    type: "edits",
    count: 9,
    adds: 66,
    dels: 77,
    files: [
      { path: "internal/handler/provider_raw_proxy.go", adds: 13, dels: 1 },
      { path: "internal/handler/helpers.go", adds: 15, dels: 24 },
      { path: "internal/handler/connections.go", adds: 1, dels: 1 },
    ],
    moreFiles: [
      { path: "internal/handler/provider_raw_proxy_paths.go", adds: 18, dels: 0 },
      { path: "internal/handler/provider_raw_proxy_test.go", adds: 9, dels: 22 },
      { path: "internal/integrations/glitchtip.go", adds: 4, dels: 11 },
      { path: "internal/integrations/admin_types.go", adds: 2, dels: 6 },
      { path: "cmd/server/serve_routes_connect.go", adds: 3, dels: 8 },
      { path: "internal/handler/employee_outbound_webhooks.go", adds: 1, dels: 4 },
    ],
  },
  {
    type: "assistant",
    text: "The previously failing GitHub job is now green on the new run. The overall lint run was still in progress when checked; unrelated `golangci-lint` and `go vet` were still running. Here are the updated review panel and settings page, plus a short walkthrough recording:",
  },
  {
    type: "attachments",
    items: [
      {
        id: "att-review",
        filename: "review-panel.png",
        kind: "image",
        url: "/samples/review-panel.png",
      },
      {
        id: "att-settings",
        filename: "settings-page.png",
        kind: "image",
        url: "/samples/settings-page.png",
      },
      {
        id: "att-walkthrough",
        filename: "walkthrough.mp4",
        kind: "video",
        url: "/samples/walkthrough.mp4",
        poster: "/samples/walkthrough-poster.png",
        duration: "0:06",
      },
    ],
  },
  {
    type: "edits",
    count: 2,
    adds: 31,
    dels: 25,
    files: [
      { path: "internal/handler/provider_raw_proxy.go", adds: 0, dels: 25 },
      { path: "internal/handler/provider_raw_proxy_paths.go", adds: 31, dels: 0 },
    ],
  },
  { type: "actions" },
]

// Sample unified diffs rendered by the Review panel via @pierre/diffs.
// PatchDiff renders exactly one file per patch, so each file is its own
// entry. Every context line must start with a single space (including
// blank ones) or the diff parser rejects the patch.
const proxyPatch = [
  "diff --git a/internal/handler/provider_raw_proxy.go b/internal/handler/provider_raw_proxy.go",
  "index 3f1c2aa..9b04e11 100644",
  "--- a/internal/handler/provider_raw_proxy.go",
  "+++ b/internal/handler/provider_raw_proxy.go",
  "@@ -10,20 +10,13 @@ import (",
  ' \t"net/http"',
  ' \t"strings"',
  " ",
  ' \t"github.com/usehivy/hivy/internal/integrations"',
  " )",
  " ",
  "-func rawProxyPathFor(provider, path string) (string, error) {",
  '-\tif strings.HasPrefix(path, "/api/0/") {',
  "-\t\treturn path, nil",
  "-\t}",
  '-\treturn "", fmt.Errorf("unsupported raw proxy path: %s", path)',
  "-}",
  "-",
  " func (h *Handler) ProviderRawProxy(w http.ResponseWriter, r *http.Request) {",
  ' \tprovider := r.PathValue("provider")',
  "-\tpath, err := rawProxyPathFor(provider, r.URL.Path)",
  "+\tpath, err := resolveRawProxyPath(provider, r.URL.Path)",
  " \tif err != nil {",
  " \t\twriteError(w, http.StatusBadGateway, err)",
  " \t\treturn",
  " \t}",
  "@@ -38,6 +31,8 @@ func (h *Handler) ProviderRawProxy(w http.ResponseWriter, r *http.Request) {",
  ' \tif provider == "glitchtip" {',
  '-\t\treq.Header.Set("Authorization", "Bearer "+nangoToken)',
  '+\t\tbaseURL := conn.Config["baseUrl"]',
  '+\t\treq.Header.Set("Authorization", "Bearer "+conn.Credentials["apiKey"])',
  "+\t\treq.URL = mustJoinURL(baseURL, path)",
  " \t}",
  " ",
  " \th.forward(w, req)",
  " }",
  "",
].join("\n")

const pathsPatch = [
  "diff --git a/internal/handler/provider_raw_proxy_paths.go b/internal/handler/provider_raw_proxy_paths.go",
  "new file mode 100644",
  "index 0000000..77ac3d2",
  "--- /dev/null",
  "+++ b/internal/handler/provider_raw_proxy_paths.go",
  "@@ -0,0 +1,17 @@",
  "+package handler",
  "+",
  "+import (",
  '+\t"fmt"',
  '+\t"strings"',
  "+)",
  "+",
  "+// resolveRawProxyPath validates and normalizes provider raw proxy paths.",
  "+func resolveRawProxyPath(provider, path string) (string, error) {",
  '+\tif strings.HasPrefix(path, "/api/0/") {',
  "+\t\treturn path, nil",
  "+\t}",
  '+\tif provider == "glitchtip" && strings.HasPrefix(path, "/internal/") {',
  '+\t\treturn strings.TrimPrefix(path, "/internal"), nil',
  "+\t}",
  '+\treturn "", fmt.Errorf("unsupported raw proxy path: %s", path)',
  "+}",
  "",
].join("\n")

export const reviewPatches = [proxyPatch, pathsPatch]

export const terminalLines: { kind: "cmd" | "out" | "ok" | "err"; text: string }[] = [
  { kind: "cmd", text: "./scripts/check-go-file-length.sh" },
  { kind: "ok", text: "OK: all Go files are within the 300 line limit" },
  { kind: "cmd", text: "go test ./internal/handler" },
  { kind: "out", text: "ok      github.com/usehivy/hivy/internal/handler   2.418s" },
  { kind: "cmd", text: "pnpm -C apps/web typecheck" },
  { kind: "out", text: "> @usehivy/web@0.0.1 typecheck" },
  { kind: "out", text: "> tsc --noEmit" },
  { kind: "cmd", text: "git push origin main" },
  { kind: "out", text: "To github.com:usehivy/hivy.git" },
  { kind: "ok", text: "   8295449d..a1f02c3e  main -> main" },
  { kind: "cmd", text: "" },
]

export interface FileTreeNode {
  name: string
  children?: FileTreeNode[]
  open?: boolean
  active?: boolean
}

export const fileTree: FileTreeNode[] = [
  {
    name: "internal",
    open: true,
    children: [
      {
        name: "handler",
        open: true,
        children: [
          { name: "connections.go" },
          { name: "helpers.go" },
          { name: "provider_raw_proxy.go", active: true },
          { name: "provider_raw_proxy_paths.go" },
          { name: "provider_raw_proxy_test.go" },
        ],
      },
      {
        name: "integrations",
        children: [{ name: "admin_types.go" }, { name: "glitchtip.go" }],
      },
    ],
  },
  {
    name: "apps",
    children: [{ name: "web" }],
  },
  { name: "cmd", children: [{ name: "server" }] },
]

export const fileContentsByPath: Record<string, string[]> = {
  "internal/handler/provider_raw_proxy_paths.go": [
    "package handler",
    "",
    "import (",
    "\t\"fmt\"",
    "\t\"strings\"",
    ")",
    "",
    "// resolveRawProxyPath validates and normalizes provider raw proxy paths.",
    "func resolveRawProxyPath(provider, path string) (string, error) {",
    "\tif strings.HasPrefix(path, \"/api/0/\") {",
    "\t\treturn path, nil",
    "\t}",
    "\tif provider == \"glitchtip\" && strings.HasPrefix(path, \"/internal/\") {",
    "\t\treturn strings.TrimPrefix(path, \"/internal\"), nil",
    "\t}",
    "\treturn \"\", fmt.Errorf(\"unsupported raw proxy path: %s\", path)",
    "}",
  ],
  "internal/handler/connections.go": [
    "package handler",
    "",
    "// Active returns the active connection for the given provider in the",
    "// caller's organization, or an error when none is configured.",
    "func (s *ConnectionStore) Active(ctx context.Context, provider string) (*Connection, error) {",
    "\tconn, err := s.repo.FindActive(ctx, provider)",
    "\tif err != nil {",
    "\t\treturn nil, fmt.Errorf(\"no active %s connection: %w\", provider, err)",
    "\t}",
    "\treturn conn, nil",
    "}",
  ],
}

export const fileContents = {
  path: "internal/handler/provider_raw_proxy.go",
  lines: [
    "package handler",
    "",
    "import (",
    "\t\"net/http\"",
    "\t\"strings\"",
    "",
    "\t\"github.com/usehivy/hivy/internal/integrations\"",
    ")",
    "",
    "// ProviderRawProxy forwards raw provider API calls through the",
    "// connection credentials stored for the active organization.",
    "func (h *Handler) ProviderRawProxy(w http.ResponseWriter, r *http.Request) {",
    "\tprovider := r.PathValue(\"provider\")",
    "\tpath, err := resolveRawProxyPath(provider, r.URL.Path)",
    "\tif err != nil {",
    "\t\twriteError(w, http.StatusBadGateway, err)",
    "\t\treturn",
    "\t}",
    "",
    "\tconn, err := h.Connections.Active(r.Context(), provider)",
    "\tif err != nil {",
    "\t\twriteError(w, http.StatusBadGateway, err)",
    "\t\treturn",
    "\t}",
    "",
    "\treq := r.Clone(r.Context())",
    "\tif provider == \"glitchtip\" {",
    "\t\tbaseURL := conn.Config[\"baseUrl\"]",
    "\t\treq.Header.Set(\"Authorization\", \"Bearer \"+conn.Credentials[\"apiKey\"])",
    "\t\treq.URL = mustJoinURL(baseURL, path)",
    "\t}",
    "",
    "\th.forward(w, req)",
    "}",
  ],
}

export const sideChatMessages: { role: "user" | "assistant"; text: string }[] = [
  {
    role: "user",
    text: "Why did the proxy tests pass but prod still 404?",
  },
  {
    role: "assistant",
    text: "The tests only assert that Hivy forwards `/api/0/*` to Nango's proxy endpoint. They never exercise the stored `connection_config.baseUrl`, so a bad base URL in the prod connection still passes CI.",
  },
  {
    role: "user",
    text: "So bypassing Nango for GlitchTip is safe?",
  },
  {
    role: "assistant",
    text: "Yes — only GlitchTip switches to the direct path. Vercel, Slack and the rest stay on the existing raw proxy, and the stored `credentials.apiKey` never leaves the server.",
  },
]
