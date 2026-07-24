#!/usr/bin/env node

import { readFileSync, readdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const dashboardDir = resolve(root, "kubernetes/observability/dashboards");
const files = readdirSync(dashboardDir).filter((name) => name.startsWith("hivy-") && name.endsWith(".json"));
const seenUIDs = new Set();
const failures = [];

for (const file of files) {
  const dashboard = JSON.parse(readFileSync(resolve(dashboardDir, file), "utf8"));
  if (!dashboard.uid || seenUIDs.has(dashboard.uid)) failures.push(`${file}: missing or duplicate uid`);
  seenUIDs.add(dashboard.uid);
  if (!dashboard.title || !Array.isArray(dashboard.panels) || dashboard.panels.length === 0) {
    failures.push(`${file}: missing title or panels`);
  }

  const panelIDs = new Set();
  for (const panel of dashboard.panels ?? []) {
    if (panelIDs.has(panel.id)) failures.push(`${file}: duplicate panel id ${panel.id}`);
    panelIDs.add(panel.id);
    if (panel.gridPos && panel.gridPos.x + panel.gridPos.w > 24) {
      failures.push(`${file}: panel ${panel.id} exceeds the 24-column grid`);
    }
    for (const target of panel.targets ?? []) {
      const sql = target.rawSql ?? "";
      if (sql && /\b(from|join)\s+public\.(?!observability_)/i.test(sql)) {
        failures.push(`${file}: panel ${panel.id} reads a non-observability table`);
      }
      if (sql && /\b(message_text|payload|text_body|html_body|encrypted_|key_hash|token_hash)\b/i.test(sql)) {
        failures.push(`${file}: panel ${panel.id} references a content or secret column`);
      }
    }
  }
}

if (failures.length > 0) {
  for (const failure of failures) console.error(failure);
  process.exit(1);
}

console.log(`validated ${files.length} Hivy dashboards`);
