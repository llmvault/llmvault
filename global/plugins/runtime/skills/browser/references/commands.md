# Command Reference

Complete reference for all browser commands. For quick start and common patterns, see SKILL.md.

## Navigation

```bash
browser open            # Launch browser (no navigation); stays on about:blank.
                              # Pair with `network route`, `cookies set --curl`, or
                              # `addinitscript` to stage state before the first navigation.
browser open <url>      # Launch + navigate (aliases: goto, navigate)
                              # Supports: https://, http://, file://, about:, data://
                              # Auto-prepends https:// if no protocol given
browser back            # Go back
browser forward         # Go forward
browser reload          # Reload page
browser pushstate <url> # SPA client-side navigation. Auto-detects
                              # window.next.router.push (triggers RSC fetch on Next.js);
                              # falls back to history.pushState + popstate/navigate events.
browser close           # Close browser (aliases: quit, exit)
browser connect 9222    # Connect to browser via CDP port
```

### Pre-navigation setup (one-turn batch)

```bash
browser batch \
  '["open"]' \
  '["network","route","*","--abort","--resource-type","script"]' \
  '["cookies","set","--curl","cookies.curl","--domain","localhost"]' \
  '["navigate","http://localhost:3000/target"]'
```

`open` with no URL gives you a clean launch so any interception, cookies,
or init scripts you register take effect on the *first* real navigation.
Use for SSR-only debug (`--resource-type script`), protected-origin auth,
or capturing fresh `react suspense`/`vitals` state without noise from a
prior page.

## Snapshot (page analysis)

```bash
browser snapshot            # Full accessibility tree
browser snapshot -i         # Interactive elements only (recommended)
browser snapshot -c         # Compact output
browser snapshot -d 3       # Limit depth to 3
browser snapshot -s "#main" # Scope to CSS selector
```

## Interactions (use @refs from snapshot)

```bash
browser click @e1           # Click
browser click @e1 --new-tab # Click and open in new tab
browser dblclick @e1        # Double-click
browser focus @e1           # Focus element
browser fill @e2 "text"     # Clear and type
browser type @e2 "text"     # Type without clearing
browser press Enter         # Press key (alias: key)
browser press Control+a     # Key combination
browser keydown Shift       # Hold key down
browser keyup Shift         # Release key
browser hover @e1           # Hover
browser check @e1           # Check checkbox
browser uncheck @e1         # Uncheck checkbox
browser select @e1 "value"  # Select dropdown option
browser select @e1 "a" "b"  # Select multiple options
browser scroll down 500     # Scroll page (default: down 300px)
browser scrollintoview @e1  # Scroll element into view (alias: scrollinto)
browser drag @e1 @e2        # Drag and drop
browser upload @e1 file.pdf # Upload files
```

Clicks fail before dispatch when another element covers the target's click
point. The error names the covering element, for example
`covered by <div#consent-banner>`. Dismiss or interact with that element, run a
fresh snapshot, then retry the original action.

## Get Information

```bash
browser get text @e1        # Get element text
browser get html @e1        # Get innerHTML
browser get value @e1       # Get input value
browser get attr @e1 href   # Get attribute
browser get title           # Get page title
browser get url             # Get current URL
browser get cdp-url         # Get CDP WebSocket URL
browser get count ".item"   # Count matching elements
browser get box @e1         # Get bounding box
browser get styles @e1      # Get computed styles (font, color, bg, etc.)
```

## Check State

```bash
browser is visible @e1      # Check if visible
browser is enabled @e1      # Check if enabled
browser is checked @e1      # Check if checked
```

## Screenshots and PDF

```bash
browser screenshot          # Save to temporary directory
browser screenshot path.png # Save to specific path
browser screenshot --full   # Full page
browser pdf output.pdf      # Save as PDF
```

Headless Chromium screenshots hide native scrollbars for consistent image output.
Pass `--hide-scrollbars false` when launching to keep native scrollbars visible.

## Video Recording

```bash
browser open https://example.com     # Launch a browser session first
browser record start ./demo.webm    # Start recording
browser click @e1                   # Perform actions
browser record stop                 # Stop and save video
browser record restart ./take2.webm # Stop current + start new
```

## Wait

```bash
browser wait @e1                     # Wait for element
browser wait 2000                    # Wait milliseconds
browser wait --text "Success"        # Wait for text (or -t)
browser wait --url "**/dashboard"    # Wait for URL pattern (or -u)
browser wait --load networkidle      # Wait for network idle (or -l)
browser wait --fn "window.ready"     # Wait for JS condition (or -f)
```

## Mouse Control

```bash
browser mouse move 100 200      # Move mouse
browser mouse down left         # Press button
browser mouse up left           # Release button
browser mouse wheel 100         # Scroll wheel
```

## Semantic Locators (alternative to refs)

```bash
browser find role button click --name "Submit"
browser find text "Sign In" click
browser find text "Sign In" click --exact      # Exact match only
browser find label "Email" fill "user@test.com"
browser find placeholder "Search" type "query"
browser find alt "Logo" click
browser find title "Close" click
browser find testid "submit-btn" click
browser find first ".item" click
browser find last ".item" click
browser find nth 2 "a" hover
```

## Browser Settings

```bash
browser set viewport 1920 1080          # Set viewport size
browser set viewport 1920 1080 2        # 2x retina (same CSS size, higher res screenshots)
browser set device "iPhone 14"          # Emulate device
browser set geo 37.7749 -122.4194       # Set geolocation (alias: geolocation)
browser set offline on                  # Toggle offline mode
browser set headers '{"X-Key":"v"}'     # Extra HTTP headers
browser set credentials user pass       # HTTP basic auth (alias: auth)
browser set media dark                  # Emulate color scheme
browser set media light reduced-motion  # Light mode + reduced motion
```

## Cookies and Storage

```bash
browser cookies                     # Get all cookies
browser cookies set name value      # Set cookie
browser cookies clear               # Clear cookies
browser storage local               # Get all localStorage
browser storage local key           # Get specific key
browser storage local set k v       # Set value
browser storage local clear         # Clear all
```

## Network

```bash
browser network route <url>              # Intercept requests
browser network route <url> --abort      # Block requests
browser network route <url> --body '{}'  # Mock response
browser network unroute [url]            # Remove routes
browser network requests                 # View tracked requests
browser network requests --filter api    # Filter requests
```

## Tabs and Windows

```bash
browser tab                              # List tabs with tabId and label
browser tab new [url]                    # New tab
browser tab new --label docs [url]       # New tab with a memorable label
browser tab t2                           # Switch to tab by id
browser tab docs                         # Switch to tab by label
browser tab close                        # Close current tab
browser tab close t2                     # Close tab by id
browser tab close docs                   # Close tab by label
browser window new                       # New window
```

Tab ids are stable strings of the form `t1`, `t2`, `t3`. They're never reused
within a session, so the same id keeps referring to the same tab across
commands. Positional integers are **not** accepted — `tab 2` errors with a
teaching message; use `t2`.

User-assigned labels (`docs`, `app`, `admin`) are interchangeable with ids
everywhere a tab ref is accepted. Labels are the agent-friendly way to write
multi-tab workflows:

```bash
browser tab new --label docs https://docs.example.com
browser tab new --label app  https://app.example.com
browser tab docs                   # switch to docs
browser snapshot                   # populate refs for docs
browser click @e1                  # ref click on docs
browser tab app                    # switch to app
browser tab close docs             # close by label
```

Labels are never auto-generated, never rewritten on navigation, and must be
unique within a session. To interact with another tab, switch to it first:
the daemon maintains a single active tab, so refs (`@eN`) belong to the tab
that was active when the snapshot ran.

## Frames

```bash
browser frame "#iframe"     # Switch to iframe by CSS selector
browser frame @e3           # Switch to iframe by element ref
browser frame main          # Back to main frame
```

### Iframe support

Iframes are detected automatically during snapshots. When the main-frame snapshot runs, `Iframe` nodes are resolved and their content is inlined beneath the iframe element in the output (one level of nesting; iframes within iframes are not expanded).

```bash
browser snapshot -i
# @e3 [Iframe] "payment-frame"
#   @e4 [input] "Card number"
#   @e5 [button] "Pay"

# Interact directly — refs inside iframes already work
browser fill @e4 "4111111111111111"
browser click @e5

# Or switch frame context for scoped snapshots
browser frame @e3               # Switch using element ref
browser snapshot -i             # Snapshot scoped to that iframe
browser frame main              # Return to main frame
```

The `frame` command accepts:
- **Element refs** — `frame @e3` resolves the ref to an iframe element
- **CSS selectors** — `frame "#payment-iframe"` finds the iframe by selector
- **Frame name/URL** — matches against the browser's frame tree

## Dialogs

By default, `alert` and `beforeunload` dialogs are automatically accepted so they never block the agent. `confirm` and `prompt` dialogs still require explicit handling. Use `--no-auto-dialog` to disable this behavior.

```bash
browser dialog accept [text]  # Accept dialog
browser dialog dismiss        # Dismiss dialog
browser dialog status         # Check if a dialog is currently open
```

## JavaScript

```bash
browser eval "document.title"          # Simple expressions only
browser eval -b "<base64>"             # Any JavaScript (base64 encoded)
browser eval --stdin                   # Read script from stdin
```

Use `-b`/`--base64` or `--stdin` for reliable execution. Shell escaping with nested quotes and special characters is error-prone.

```bash
# Base64 encode your script, then:
browser eval -b "ZG9jdW1lbnQucXVlcnlTZWxlY3RvcignW3NyYyo9Il9uZXh0Il0nKQ=="

# Or use stdin with heredoc for multiline scripts:
cat <<'EOF' | browser eval --stdin
const links = document.querySelectorAll('a');
Array.from(links).map(a => a.href);
EOF
```

## Authentication and Plugins

```bash
browser auth save <name> --url <url> --username <user> --password-stdin
browser auth login <name>          # Login using saved credentials
browser auth login <name> --credential-provider <plugin> [--item <ref>] [--url <url>]
browser auth login <name> --username-selector <s> --password-selector <s> [--submit-selector <s>]
browser auth list                  # List saved auth profiles
browser auth show <name>           # Show profile metadata, no passwords
browser auth delete <name>         # Delete a saved profile
browser plugin add <ref>           # Add a configured plugin reference
browser plugin list                # List configured plugins
browser plugin show <name>         # Show one configured plugin
browser plugin run <name> <type> --payload <json>
                                          # Run an arbitrary plugin request
```

Credential provider plugins run out-of-process over the
`browser.plugin.v1` stdio JSON protocol and must declare
`credential.read`. Use `--confirm-actions plugin:<name>:credential.read`
to require explicit approval before a plugin resolves secrets.

Other capabilities use the same protocol:
- `browser.provider`: `browser --provider <name> open <url>`
- `launch.mutate`: append local launch args, extensions, or init scripts
- `command.run`: `browser plugin run <name> <type> --payload <json>`

`plugin run` is for `command.run` and custom capabilities. Core capabilities
and protocol request types use their dedicated command paths.

## State Management

```bash
browser state save auth.json    # Save cookies, storage, auth state
browser state load auth.json    # Restore saved state
```

## Global Options

```bash
browser --session <name> ...    # Isolated browser session
browser --json ...              # JSON output for parsing
browser --headed ...            # Show browser window (not headless)
browser --cdp <port> ...        # Connect via Chrome DevTools Protocol
browser -p <provider> ...       # Browser provider or configured provider plugin
browser --proxy <url> ...       # Use proxy server
browser --proxy-bypass <hosts>  # Hosts to bypass proxy
browser --headers <json> ...    # HTTP headers scoped to URL's origin
browser --executable-path <p>   # Custom browser executable
browser --extension <path> ...  # Load browser extension (repeatable)
browser --ignore-https-errors   # Ignore SSL certificate errors
browser --hide-scrollbars false # Keep native scrollbars visible in headless Chromium screenshots
browser --help                  # Show help (-h)
browser --version               # Show version (-V)
browser <command> --help        # Show detailed help for a command
```

## Debugging

```bash
browser --headed open example.com   # Show browser window
browser --cdp 9222 snapshot         # Connect via CDP port
browser connect 9222                # Alternative: connect command
browser console                     # View console messages
browser console --clear             # Clear console
browser errors                      # View page errors
browser errors --clear              # Clear errors
browser highlight @e1               # Highlight element
browser inspect                     # Open Chrome DevTools for this session
browser trace start                 # Start recording trace
browser trace stop trace.json       # Stop and save trace
browser profiler start              # Start Chrome DevTools profiling
browser profiler stop trace.json    # Stop and save profile
```

## React / Web Vitals

Requires `--enable react-devtools` at launch for the `react ...` commands.
`vitals` and `pushstate` are framework-agnostic.

```bash
browser open --enable react-devtools <url>    # Launch with React hook enabled
browser react tree                            # Full component tree
browser react inspect <fiberId>               # Props, hooks, state, source
browser react renders start                   # Begin re-render recording
browser react renders stop [--json]           # Stop and print render profile
browser react suspense [--only-dynamic] [--json]  # Suspense boundaries + classifier
                                                         # --only-dynamic hides the "static" list
browser vitals [url] [--json]                 # LCP/CLS/TTFB/FCP/INP + hydration
browser pushstate <url>                       # SPA client-side nav (auto-detects Next router)
```

`vitals` prints a summary by default and uses the same fields as the structured
`--json` response.

## Init scripts

```bash
browser open --init-script <path>             # Register before first navigation (repeatable)
browser addinitscript <js>                    # Register at runtime (returns identifier)
browser removeinitscript <identifier>         # Remove a previously registered init script
```

## cURL cookie import

```bash
browser cookies set --curl <file>                             # Auto-detects JSON/cURL/Cookie-header
browser cookies set --curl <file> --domain example.com        # Scope to a domain
```

Supported formats: JSON array of `{name, value}`, a cURL dump from
DevTools -> Network -> Copy as cURL, or a bare Cookie header. Errors never
echo cookie values.

## Network route by resource type

```bash
browser network route '*' --abort --resource-type script       # Block scripts only (SSR-lock pattern)
browser network route '*' --resource-type image,font --body '' # Stub images and fonts
```

## Environment Variables

```bash
AGENT_BROWSER_SESSION="mysession"            # Default session name
AGENT_BROWSER_EXECUTABLE_PATH="/path/chrome" # Custom browser path
AGENT_BROWSER_EXTENSIONS="/ext1,/ext2"       # Comma-separated extension paths
AGENT_BROWSER_INIT_SCRIPTS="/a.js,/b.js"     # Comma-separated init script paths
AGENT_BROWSER_ENABLE="react-devtools"        # Comma-separated built-in init script features
AGENT_BROWSER_HIDE_SCROLLBARS="false"        # Keep native scrollbars visible in headless Chromium screenshots
AGENT_BROWSER_PROVIDER="browserbase"         # Browser provider or configured provider plugin
AGENT_BROWSER_STREAM_PORT="9223"             # Override WebSocket streaming port (default: OS-assigned)
AGENT_BROWSER_CONFIG="./browser.json"  # Custom config file
AGENT_BROWSER_CDP="9222"                     # Connect daemon to CDP port or WebSocket URL
AGENT_BROWSER_PLUGINS='[{"name":"vault","command":"browser-plugin-vault","capabilities":["credential.read"]},{"name":"stealth","command":"browser-plugin-stealth","capabilities":["launch.mutate"]}]'
```
