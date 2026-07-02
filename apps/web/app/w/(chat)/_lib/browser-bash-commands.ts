type BrowserCommandText = string | ((match: RegExpMatchArray) => string)

export interface BrowserBashCommandPattern {
  id: string
  pattern: RegExp
  label: BrowserCommandText
  runningLabel: BrowserCommandText
  actionIcon: BrowserCommandText
}

export interface BrowserBashCommandMatch {
  id: string
  kind: "Chrome browser"
  icon: "chrome"
  actionIcon: string
  label: string
  runningLabel: string
}

const shellToken = String.raw`(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s;&|()]+)`
const browserValueOption = String.raw`--(?:session|session-name|state|profile|provider|proxy|proxy-bypass|headers|executable-path|extension|init-script|cdp|confirm-actions|enable|hide-scrollbars)(?:[=\s]${shellToken})?`
const browserFlagOption = String.raw`--(?:json|headed|ignore-https-errors|auto-connect|help|version)|-[hV]`
const browserShortOption = String.raw`-[pc]\s+${shellToken}`
const browserInvocation = String.raw`(?:^|[^A-Za-z0-9_./-])browser(?:\s+(?:${browserValueOption}|${browserFlagOption}|${browserShortOption}))*\s+`
const commandRest = String.raw`(?:\s+[^;&|]*)?`

function browserPattern(command: string): RegExp {
  return new RegExp(`${browserInvocation}${command}`, "ims")
}

function commandLabel(action: string) {
  return `${action[0]?.toUpperCase() ?? ""}${action.slice(1)} matching element`
}

function runningCommandLabel(action: string) {
  const present = action.endsWith("ed") ? action.slice(0, -2) : action
  return `${present[0]?.toUpperCase() ?? ""}${present.slice(1)}ing matching element`
}

function semanticLabel(match: RegExpMatchArray) {
  const locator = match[1] || "element"
  const action = match[2] || "click"
  const locatorLabel = locator === "testid" ? "test id" : locator

  switch (action) {
    case "click":
      return `Clicked matching ${locatorLabel}`
    case "dblclick":
      return `Double-clicked matching ${locatorLabel}`
    case "fill":
      return `Filled matching ${locatorLabel}`
    case "type":
      return `Typed into matching ${locatorLabel}`
    case "hover":
      return `Hovered matching ${locatorLabel}`
    case "focus":
      return `Focused matching ${locatorLabel}`
    case "select":
      return `Selected matching ${locatorLabel}`
    case "check":
      return `Checked matching ${locatorLabel}`
    case "uncheck":
      return `Unchecked matching ${locatorLabel}`
    default:
      return commandLabel(action)
  }
}

function semanticRunningLabel(match: RegExpMatchArray) {
  const locator = match[1] || "element"
  const action = match[2] || "click"
  const locatorLabel = locator === "testid" ? "test id" : locator

  switch (action) {
    case "click":
      return `Clicking matching ${locatorLabel}`
    case "dblclick":
      return `Double-clicking matching ${locatorLabel}`
    case "fill":
      return `Filling matching ${locatorLabel}`
    case "type":
      return `Typing into matching ${locatorLabel}`
    case "hover":
      return `Hovering matching ${locatorLabel}`
    case "focus":
      return `Focusing matching ${locatorLabel}`
    case "select":
      return `Selecting matching ${locatorLabel}`
    case "check":
      return `Checking matching ${locatorLabel}`
    case "uncheck":
      return `Unchecking matching ${locatorLabel}`
    default:
      return runningCommandLabel(action)
  }
}

function semanticActionIcon(match: RegExpMatchArray) {
  switch (match[2] || "click") {
    case "fill":
    case "type":
      return "text-cursor-input"
    case "hover":
      return "mouse-pointer-2"
    case "focus":
      return "focus"
    case "select":
      return "list-checks"
    case "check":
      return "square-check"
    case "uncheck":
      return "square"
    default:
      return "mouse-pointer-click"
  }
}

export const BROWSER_BASH_COMMAND_PATTERNS: BrowserBashCommandPattern[] = [
  {
    id: "semantic_locator_action",
    pattern: browserPattern(
      String.raw`find\s+(role|text|label|placeholder|alt|title|testid|first|last|nth|row)\b[^;&|\n]*?\b(click|dblclick|fill|type|hover|focus|select|check|uncheck)\b`
    ),
    label: semanticLabel,
    runningLabel: semanticRunningLabel,
    actionIcon: semanticActionIcon,
  },
  {
    id: "open",
    pattern: browserPattern(String.raw`(?:open|goto|navigate)\b`),
    label: "Opened Chrome browser",
    runningLabel: "Opening Chrome browser",
    actionIcon: "external-link",
  },
  {
    id: "install",
    pattern: browserPattern(String.raw`install\b`),
    label: "Installed Chrome browser",
    runningLabel: "Installing Chrome browser",
    actionIcon: "download",
  },
  {
    id: "snapshot",
    pattern: browserPattern(String.raw`snapshot\b`),
    label: "Took a page snapshot",
    runningLabel: "Taking a page snapshot",
    actionIcon: "scan-search",
  },
  {
    id: "click",
    pattern: browserPattern(String.raw`click\b`),
    label: "Clicked an element",
    runningLabel: "Clicking an element",
    actionIcon: "mouse-pointer-click",
  },
  {
    id: "double_click",
    pattern: browserPattern(String.raw`dblclick\b`),
    label: "Double-clicked an element",
    runningLabel: "Double-clicking an element",
    actionIcon: "mouse-pointer-click",
  },
  {
    id: "hover",
    pattern: browserPattern(String.raw`hover\b`),
    label: "Hovered an element",
    runningLabel: "Hovering an element",
    actionIcon: "mouse-pointer-2",
  },
  {
    id: "focus",
    pattern: browserPattern(String.raw`focus\b`),
    label: "Focused an element",
    runningLabel: "Focusing an element",
    actionIcon: "focus",
  },
  {
    id: "fill",
    pattern: browserPattern(String.raw`fill\b`),
    label: "Filled an input",
    runningLabel: "Filling an input",
    actionIcon: "text-cursor-input",
  },
  {
    id: "type",
    pattern: browserPattern(String.raw`type\b`),
    label: "Typed into an input",
    runningLabel: "Typing into an input",
    actionIcon: "keyboard",
  },
  {
    id: "press",
    pattern: browserPattern(String.raw`(?:press|key)\b`),
    label: "Pressed a key",
    runningLabel: "Pressing a key",
    actionIcon: "keyboard",
  },
  {
    id: "check",
    pattern: browserPattern(String.raw`check\b`),
    label: "Checked a checkbox",
    runningLabel: "Checking a checkbox",
    actionIcon: "square-check",
  },
  {
    id: "uncheck",
    pattern: browserPattern(String.raw`uncheck\b`),
    label: "Unchecked a checkbox",
    runningLabel: "Unchecking a checkbox",
    actionIcon: "square",
  },
  {
    id: "select",
    pattern: browserPattern(String.raw`select\b`),
    label: "Selected an option",
    runningLabel: "Selecting an option",
    actionIcon: "list-checks",
  },
  {
    id: "upload",
    pattern: browserPattern(String.raw`upload\b`),
    label: "Uploaded a file",
    runningLabel: "Uploading a file",
    actionIcon: "upload",
  },
  {
    id: "scroll",
    pattern: browserPattern(String.raw`scroll\b`),
    label: "Scrolled the page",
    runningLabel: "Scrolling the page",
    actionIcon: "scroll",
  },
  {
    id: "scroll_into_view",
    pattern: browserPattern(String.raw`(?:scrollintoview|scrollinto)\b`),
    label: "Scrolled to an element",
    runningLabel: "Scrolling to an element",
    actionIcon: "move-vertical",
  },
  {
    id: "drag",
    pattern: browserPattern(String.raw`drag\b`),
    label: "Dragged an element",
    runningLabel: "Dragging an element",
    actionIcon: "move",
  },
  {
    id: "wait",
    pattern: browserPattern(String.raw`wait\b`),
    label: "Waited for page state",
    runningLabel: "Waiting for page state",
    actionIcon: "clock",
  },
  {
    id: "get_text",
    pattern: browserPattern(String.raw`get\s+text\b`),
    label: "Read element text",
    runningLabel: "Reading element text",
    actionIcon: "text-search",
  },
  {
    id: "get_html",
    pattern: browserPattern(String.raw`get\s+html\b`),
    label: "Read element HTML",
    runningLabel: "Reading element HTML",
    actionIcon: "code-xml",
  },
  {
    id: "get_value",
    pattern: browserPattern(String.raw`get\s+value\b`),
    label: "Read input value",
    runningLabel: "Reading input value",
    actionIcon: "text-cursor-input",
  },
  {
    id: "get_attr",
    pattern: browserPattern(String.raw`get\s+attr\b`),
    label: "Read an element attribute",
    runningLabel: "Reading an element attribute",
    actionIcon: "braces",
  },
  {
    id: "get_title",
    pattern: browserPattern(String.raw`get\s+title\b`),
    label: "Read page title",
    runningLabel: "Reading page title",
    actionIcon: "heading",
  },
  {
    id: "get_url",
    pattern: browserPattern(String.raw`get\s+url\b`),
    label: "Read current URL",
    runningLabel: "Reading current URL",
    actionIcon: "link",
  },
  {
    id: "get_count",
    pattern: browserPattern(String.raw`get\s+count\b`),
    label: "Counted page elements",
    runningLabel: "Counting page elements",
    actionIcon: "list-ordered",
  },
  {
    id: "get_box",
    pattern: browserPattern(String.raw`get\s+box\b`),
    label: "Measured an element",
    runningLabel: "Measuring an element",
    actionIcon: "box-select",
  },
  {
    id: "get_styles",
    pattern: browserPattern(String.raw`get\s+styles\b`),
    label: "Read element styles",
    runningLabel: "Reading element styles",
    actionIcon: "palette",
  },
  {
    id: "screenshot",
    pattern: browserPattern(String.raw`screenshot\b`),
    label: "Took a screenshot",
    runningLabel: "Taking a screenshot",
    actionIcon: "camera",
  },
  {
    id: "pdf",
    pattern: browserPattern(String.raw`pdf\b`),
    label: "Saved a PDF",
    runningLabel: "Saving a PDF",
    actionIcon: "file-text",
  },
  {
    id: "close",
    pattern: browserPattern(String.raw`(?:close|quit|exit)\b`),
    label: "Closed Chrome browser",
    runningLabel: "Closing Chrome browser",
    actionIcon: "x",
  },
  {
    id: "back",
    pattern: browserPattern(String.raw`back\b`),
    label: "Went back",
    runningLabel: "Going back",
    actionIcon: "arrow-left",
  },
  {
    id: "forward",
    pattern: browserPattern(String.raw`forward\b`),
    label: "Went forward",
    runningLabel: "Going forward",
    actionIcon: "arrow-right",
  },
  {
    id: "reload",
    pattern: browserPattern(String.raw`reload\b`),
    label: "Reloaded the page",
    runningLabel: "Reloading the page",
    actionIcon: "rotate-cw",
  },
  {
    id: "pushstate",
    pattern: browserPattern(String.raw`pushstate\b`),
    label: "Changed page route",
    runningLabel: "Changing page route",
    actionIcon: "route",
  },
  {
    id: "set",
    pattern: browserPattern(
      String.raw`set\s+(?:viewport|device|geo|geolocation|offline|headers|credentials|auth|media)\b`
    ),
    label: "Updated browser settings",
    runningLabel: "Updating browser settings",
    actionIcon: "settings",
  },
  {
    id: "cookies",
    pattern: browserPattern(String.raw`cookies\b`),
    label: "Managed browser cookies",
    runningLabel: "Managing browser cookies",
    actionIcon: "cookie",
  },
  {
    id: "storage",
    pattern: browserPattern(String.raw`storage\b`),
    label: "Managed browser storage",
    runningLabel: "Managing browser storage",
    actionIcon: "database",
  },
  {
    id: "network",
    pattern: browserPattern(String.raw`network\s+(?:route|unroute|requests)\b`),
    label: "Managed network requests",
    runningLabel: "Managing network requests",
    actionIcon: "network",
  },
  {
    id: "tab",
    pattern: browserPattern(String.raw`tab\b`),
    label: "Managed browser tabs",
    runningLabel: "Managing browser tabs",
    actionIcon: "panel-top",
  },
  {
    id: "window",
    pattern: browserPattern(String.raw`window\s+new\b`),
    label: "Opened a browser window",
    runningLabel: "Opening a browser window",
    actionIcon: "panels-top-left",
  },
  {
    id: "frame",
    pattern: browserPattern(String.raw`frame\b`),
    label: "Switched browser frame",
    runningLabel: "Switching browser frame",
    actionIcon: "panel-top-open",
  },
  {
    id: "dialog",
    pattern: browserPattern(String.raw`dialog\s+(?:accept|dismiss|status)\b`),
    label: "Handled a browser dialog",
    runningLabel: "Handling a browser dialog",
    actionIcon: "message-square",
  },
  {
    id: "eval",
    pattern: browserPattern(String.raw`eval\b${commandRest}`),
    label: "Ran JavaScript in the page",
    runningLabel: "Running JavaScript in the page",
    actionIcon: "braces",
  },
  {
    id: "auth",
    pattern: browserPattern(
      String.raw`auth\s+(?:save|login|list|show|delete)\b`
    ),
    label: "Used browser authentication",
    runningLabel: "Using browser authentication",
    actionIcon: "key-round",
  },
  {
    id: "state_save",
    pattern: browserPattern(String.raw`state\s+save\b`),
    label: "Saved browser state",
    runningLabel: "Saving browser state",
    actionIcon: "save",
  },
  {
    id: "state_load",
    pattern: browserPattern(String.raw`state\s+load\b`),
    label: "Loaded browser state",
    runningLabel: "Loading browser state",
    actionIcon: "folder-open",
  },
  {
    id: "record",
    pattern: browserPattern(String.raw`record\s+(?:start|stop|restart)\b`),
    label: "Recorded browser video",
    runningLabel: "Recording browser video",
    actionIcon: "video",
  },
  {
    id: "console",
    pattern: browserPattern(String.raw`console\b`),
    label: "Checked browser console",
    runningLabel: "Checking browser console",
    actionIcon: "terminal",
  },
  {
    id: "errors",
    pattern: browserPattern(String.raw`errors\b`),
    label: "Checked page errors",
    runningLabel: "Checking page errors",
    actionIcon: "triangle-alert",
  },
  {
    id: "highlight",
    pattern: browserPattern(String.raw`highlight\b`),
    label: "Highlighted an element",
    runningLabel: "Highlighting an element",
    actionIcon: "highlighter",
  },
  {
    id: "inspect",
    pattern: browserPattern(String.raw`inspect\b`),
    label: "Opened DevTools",
    runningLabel: "Opening DevTools",
    actionIcon: "bug",
  },
  {
    id: "trace",
    pattern: browserPattern(String.raw`trace\s+(?:start|stop)\b`),
    label: "Captured a browser trace",
    runningLabel: "Capturing a browser trace",
    actionIcon: "activity",
  },
  {
    id: "profiler",
    pattern: browserPattern(String.raw`profiler\s+(?:start|stop)\b`),
    label: "Captured a browser profile",
    runningLabel: "Capturing a browser profile",
    actionIcon: "gauge",
  },
  {
    id: "connect",
    pattern: browserPattern(String.raw`connect\b`),
    label: "Connected to Chrome",
    runningLabel: "Connecting to Chrome",
    actionIcon: "plug",
  },
  {
    id: "batch",
    pattern: browserPattern(String.raw`batch\b`),
    label: "Ran browser command batch",
    runningLabel: "Running browser command batch",
    actionIcon: "list-tree",
  },
  {
    id: "plugin",
    pattern: browserPattern(String.raw`plugin\s+(?:add|list|show|run)\b`),
    label: "Used browser plugin",
    runningLabel: "Using browser plugin",
    actionIcon: "plug",
  },
  {
    id: "mouse",
    pattern: browserPattern(String.raw`mouse\s+(?:move|down|up|wheel)\b`),
    label: "Controlled the mouse",
    runningLabel: "Controlling the mouse",
    actionIcon: "mouse",
  },
]

export function detectBrowserBashCommand(
  command: string
): BrowserBashCommandMatch | undefined {
  if (!command.trim()) return undefined

  for (const spec of BROWSER_BASH_COMMAND_PATTERNS) {
    const match = command.match(spec.pattern)
    if (!match) continue

    return {
      id: spec.id,
      kind: "Chrome browser",
      icon: "chrome",
      actionIcon: commandText(spec.actionIcon, match),
      label: commandText(spec.label, match),
      runningLabel: commandText(spec.runningLabel, match),
    }
  }

  return undefined
}

function commandText(
  value: BrowserCommandText,
  match: RegExpMatchArray
): string {
  return typeof value === "function" ? value(match) : value
}
