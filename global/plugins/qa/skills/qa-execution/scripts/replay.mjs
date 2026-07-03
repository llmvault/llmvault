#!/usr/bin/env node
// replay.mjs — Tier-0 deterministic replay of a QA test case's cached commands.
//
// Usage: node replay.mjs <commands.json> <session-name> [artifact-dir]
//
// commands.json is the Commands cell of a test case: a JSON array of step
// objects. No prose is ever parsed; commands are argv arrays executed via
// execFile (no shell, no quoting, no eval).
//
//   [
//     { "intent": "Open the login page",
//       "command": ["open", "${QA_BASE_URL}/login"] },
//     { "intent": "Fill the email field with the QA admin email",
//       "command": ["find", "label", "Email", "fill", "${QA_ADMIN_EMAIL}"] },
//     { "intent": "URL is the workspace shell",
//       "assert": true,
//       "command": ["get", "url"],
//       "expect": { "op": "contains", "value": "/w" } },
//     { "intent": "Legal footer link (pinned)",
//       "no_heal": true,
//       "command": ["find", "role", "link", "Terms of Service", "click"] }
//   ]
//
// Step fields: intent (string, required) · command (string[], required) ·
// expect ({op: "contains"|"equals", value: string}, optional) ·
// assert (bool, marks verification steps — never healed) ·
// no_heal (bool, pinned — never healed).
// ${NAME} tokens inside command strings are replaced from the environment;
// an undefined variable is a setup error (exit 2), never a guess.
//
// stdout's final line is always one JSON verdict:
//   {"status":"passed","steps_run":N}
//   {"status":"failed","failed_step":N,"intent":"...","assert":bool,"pinned":bool,
//    "command":[...],"expect":{...}|null,"output":"...","screenshot":"..."}
//   {"status":"error","message":"..."}            (schema/setup problem)
// Exit codes: 0 passed · 1 failed · 2 error.

import { execFileSync } from "node:child_process";
import { readFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

const BROWSER_BIN = process.env.BROWSER_BIN || "browser";
const [, , commandsFile, session, artifactDir = "."] = process.argv;

function die(message) {
  console.log(JSON.stringify({ status: "error", message }));
  process.exit(2);
}

if (!commandsFile || !session) die("usage: replay.mjs <commands.json> <session-name> [artifact-dir]");

let steps;
try {
  steps = JSON.parse(readFileSync(commandsFile, "utf8"));
} catch (e) {
  die(`cannot read/parse ${commandsFile}: ${e.message}`);
}

// -- schema validation: fail fast and precisely, never misinterpret --
if (!Array.isArray(steps) || steps.length === 0) die("commands.json must be a non-empty JSON array of steps");
steps.forEach((s, i) => {
  const at = `step ${i + 1}`;
  if (typeof s !== "object" || s === null || Array.isArray(s)) die(`${at}: must be an object`);
  if (typeof s.intent !== "string" || !s.intent.trim()) die(`${at}: "intent" (non-empty string) is required`);
  if (!Array.isArray(s.command) || s.command.length === 0 || !s.command.every((a) => typeof a === "string"))
    die(`${at}: "command" must be a non-empty array of strings`);
  if (s.expect !== undefined) {
    if (typeof s.expect !== "object" || s.expect === null) die(`${at}: "expect" must be an object`);
    if (!["contains", "equals"].includes(s.expect.op)) die(`${at}: expect.op must be "contains" or "equals"`);
    if (typeof s.expect.value !== "string") die(`${at}: expect.value must be a string`);
  }
  for (const k of Object.keys(s))
    if (!["intent", "command", "expect", "assert", "no_heal"].includes(k)) die(`${at}: unknown key "${k}"`);
});

// ${NAME} substitution from the environment; undefined var = setup error.
function substitute(arg, at) {
  return arg.replace(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g, (_, name) => {
    if (process.env[name] === undefined) die(`${at}: environment variable \${${name}} is not set`);
    return process.env[name];
  });
}

mkdirSync(artifactDir, { recursive: true });

function runBrowser(argv) {
  try {
    const out = execFileSync(BROWSER_BIN, [...argv, "--session", session], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      timeout: 120_000,
    });
    return { ok: true, output: out };
  } catch (e) {
    const output = [e.stdout, e.stderr, e.code === "ETIMEDOUT" ? "(timed out)" : ""].filter(Boolean).join("\n");
    return { ok: false, output: output || e.message };
  }
}

for (let i = 0; i < steps.length; i++) {
  const s = steps[i];
  const n = i + 1;
  const argv = s.command.map((a) => substitute(a, `step ${n}`));
  const res = runBrowser(argv);

  let ok = res.ok;
  if (ok && s.expect) {
    const trimmed = res.output.replace(/\r/g, "").trim();
    ok = s.expect.op === "contains" ? res.output.includes(s.expect.value) : trimmed === s.expect.value;
  }

  if (!ok) {
    let screenshot = join(artifactDir, `step-${n}-failure.png`);
    const shot = runBrowser(["screenshot", screenshot]);
    if (!shot.ok) screenshot = "";
    console.log(
      JSON.stringify({
        status: "failed",
        failed_step: n,
        intent: s.intent,
        assert: s.assert === true,
        pinned: s.no_heal === true,
        command: argv,
        expect: s.expect ?? null,
        output: res.output.slice(0, 2000),
        screenshot,
      })
    );
    process.exit(1);
  }
}

console.log(JSON.stringify({ status: "passed", steps_run: steps.length }));
