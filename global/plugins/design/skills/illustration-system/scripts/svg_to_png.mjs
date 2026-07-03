#!/usr/bin/env node
// svg_to_png.mjs — Convert one SVG to transparent PNG(s) at a ladder of sizes.
//
// Usage:
//   node svg_to_png.mjs input.svg [--out dir] [--sizes 1024,512,256,128,64] [--background none]
//
// Transparent by default. Prefers rsvg-convert when installed, else installs
// and uses @resvg/resvg-js (into /workspace/.svgtools). Same rasterizer the
// logo_pack.mjs script uses.

import fs from 'fs';
import path from 'path';
import { execSync, execFileSync } from 'child_process';
import { createRequire } from 'module';
const require = createRequire(import.meta.url);

const argv = process.argv.slice(2);
const input = argv.find((a) => !a.startsWith('--'));
if (!input) { console.error('usage: node svg_to_png.mjs input.svg [--out dir] [--sizes 1024,512,...] [--background none]'); process.exit(2); }
if (!fs.existsSync(input)) { console.error('error: input not found: ' + input); process.exit(1); }
function opt(name, def) { const i = argv.indexOf('--' + name); return i >= 0 && argv[i + 1] ? argv[i + 1] : def; }
const OUT = opt('out', '.');
const SIZES = opt('sizes', '1024,512,256,128,64').split(',').map((s) => parseInt(s, 10)).filter(Boolean);
const BG = opt('background', 'none');
fs.mkdirSync(OUT, { recursive: true });

let hasRsvg = false;
try { execSync('command -v rsvg-convert', { stdio: 'ignore' }); hasRsvg = true; } catch {}
let Resvg = null;
function loadResvg() {
  if (Resvg) return Resvg;
  for (const t of ['@resvg/resvg-js', path.join(process.cwd(), 'node_modules/@resvg/resvg-js'), '/workspace/.svgtools/node_modules/@resvg/resvg-js']) {
    try { Resvg = require(t).Resvg; return Resvg; } catch {}
  }
  const dir = fs.existsSync('/workspace') ? '/workspace/.svgtools' : path.join(process.cwd(), '.svgtools');
  console.error(`installing @resvg/resvg-js into ${dir} (one-time)...`);
  fs.mkdirSync(dir, { recursive: true });
  execSync('npm install --no-audit --no-fund --silent @resvg/resvg-js', { cwd: dir, stdio: 'inherit' });
  Resvg = require(path.join(dir, 'node_modules/@resvg/resvg-js')).Resvg;
  return Resvg;
}

const base = path.basename(input).replace(/\.svg$/i, '');
for (const w of SIZES) {
  const out = path.join(OUT, `${base}-${w}.png`);
  if (hasRsvg) {
    const a = ['-w', String(w), '-o', out];
    if (BG !== 'none') a.push('-b', BG);
    execFileSync('rsvg-convert', [...a, input]);
  } else {
    const R = loadResvg();
    const opts = { fitTo: { mode: 'width', value: w }, font: { loadSystemFonts: true } };
    if (BG !== 'none') opts.background = BG;
    fs.writeFileSync(out, new R(fs.readFileSync(input), opts).render().asPng());
  }
  console.log(`wrote ${out}`);
}
