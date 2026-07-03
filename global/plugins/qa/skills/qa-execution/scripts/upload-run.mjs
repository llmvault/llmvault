#!/usr/bin/env node
// upload-run.mjs — upload a QA run's artifact directory to the agent drive.
//
// Usage: node upload-run.mjs <artifact-dir> [remote-prefix]
//
// PUTs every non-empty regular file under <artifact-dir> to
//   $HIVY_DRIVE_UPLOAD_URL/<remote-prefix>/<relative-path>
// with bearer $HIVY_DRIVE_UPLOAD_BEARER (provided by the sandbox runtime; see
// the drive skill). remote-prefix defaults to qa/runs/<basename of dir>.
//
// stdout: one JSON object per file (JSON Lines):
//   {"file":"case-12/failure.png","key":"pub/e/.../failure.png","asset_url":"https://..."}
//   {"file":"case-13/console.log","error":"401 {\"error\":\"unauthorized\"}"}
// Final line is a summary: {"uploaded":N,"failed":N,"skipped_empty":N}
// Keys are valid sheet attachment-cell values; asset_urls render in chat.
// Exit codes: 0 all uploaded · 1 any failed · 2 usage/setup error.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, basename } from "node:path";

const [, , dir, prefixArg] = process.argv;

function die(message) {
  console.log(JSON.stringify({ uploaded: 0, failed: 0, skipped_empty: 0, error: message }));
  process.exit(2);
}

if (!dir) die("usage: upload-run.mjs <artifact-dir> [remote-prefix]");
try {
  if (!statSync(dir).isDirectory()) die(`not a directory: ${dir}`);
} catch {
  die(`not a directory: ${dir}`);
}
const base = process.env.HIVY_DRIVE_UPLOAD_URL;
const bearer = process.env.HIVY_DRIVE_UPLOAD_BEARER;
if (!base || !bearer) die("HIVY_DRIVE_UPLOAD_URL / HIVY_DRIVE_UPLOAD_BEARER not set (no sandbox drive endpoint)");
const prefix = prefixArg || `qa/runs/${basename(dir)}`;

const TYPES = {
  png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", gif: "image/gif",
  webm: "video/webm", mp4: "video/mp4", json: "application/json",
  html: "text/html", txt: "text/plain", log: "text/plain",
};
const contentType = (f) => TYPES[f.split(".").pop().toLowerCase()] ?? "application/octet-stream";

function* walk(d) {
  for (const entry of readdirSync(d, { withFileTypes: true })) {
    const p = join(d, entry.name);
    if (entry.isDirectory()) yield* walk(p);
    else if (entry.isFile()) yield p;
  }
}

let uploaded = 0, failed = 0, skippedEmpty = 0;
for (const path of walk(dir)) {
  const rel = relative(dir, path);
  const body = readFileSync(path);
  if (body.length === 0) { skippedEmpty++; continue; } // drive rejects empty files
  try {
    const res = await fetch(`${base}/${prefix}/${rel}`, {
      method: "PUT",
      headers: { Authorization: `Bearer ${bearer}`, "Content-Type": contentType(rel) },
      body,
    });
    const text = await res.text();
    let parsed = null;
    try { parsed = JSON.parse(text); } catch { /* non-JSON error body */ }
    if (res.ok && parsed?.key) {
      console.log(JSON.stringify({ file: rel, key: parsed.key, asset_url: parsed.asset_url ?? "" }));
      uploaded++;
    } else {
      console.log(JSON.stringify({ file: rel, error: `${res.status} ${text.slice(0, 200)}` }));
      failed++;
    }
  } catch (e) {
    console.log(JSON.stringify({ file: rel, error: e.message }));
    failed++;
  }
}

console.log(JSON.stringify({ uploaded, failed, skipped_empty: skippedEmpty }));
process.exit(failed === 0 ? 0 : 1);
