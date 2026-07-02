#!/usr/bin/env node
// logo_pack.mjs — Build a complete, consistent logo pack from ONE generated mark.
//
// Given a single canonical mark SVG (produced once by generate_vector_image),
// this composes every deliverable — horizontal & stacked lockups, app icon,
// monochrome, reverse, favicon — by REUSING the exact mark geometry (recolor or
// reposition only, never redraw). It then rasterizes each SVG to transparent
// PNGs and verifies that every mark path survived byte-identical.
//
// Usage:
//   node logo_pack.mjs --mark mark.svg --name "Meadowlark" --color "#3F7D4E" \
//       [--font "Georgia, serif"] [--weight 600] [--wordmark wordmark.svg] \
//       [--out logo-pack] [--sizes 1024,512,256,128,64]
//
// Only --mark is strictly required. Without --name the wordmark lockups are
// skipped (you still get mark/app-icon/mono/reverse/favicon).

import fs from 'fs';
import path from 'path';
import crypto from 'crypto';
import { execSync, execFileSync } from 'child_process';
import { createRequire } from 'module';

const require = createRequire(import.meta.url);

// ---------- args ----------
function parseArgs(argv) {
  const a = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i].startsWith('--')) {
      const k = argv[i].slice(2);
      const v = argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[++i] : 'true';
      a[k] = v;
    }
  }
  return a;
}
const args = parseArgs(process.argv.slice(2));
function die(msg) { console.error('error: ' + msg); process.exit(1); }
if (!args.mark) die('missing --mark <mark.svg>');
if (!fs.existsSync(args.mark)) die(`mark not found: ${args.mark}`);

const NAME = (args.name && args.name !== 'true') ? args.name : '';
const COLOR = (args.color && args.color !== 'true') ? args.color : '#111111';
const FONT = (args.font && args.font !== 'true') ? args.font : 'Helvetica Neue, Helvetica, Arial, sans-serif';
const WEIGHT = (args.weight && args.weight !== 'true') ? args.weight : '700';
const DARK = (args.dark && args.dark !== 'true') ? args.dark : darken(COLOR);
const OUT = (args.out && args.out !== 'true') ? args.out : 'logo-pack';
const SIZES = (args.sizes && args.sizes !== 'true') ? args.sizes.split(',').map((s) => parseInt(s, 10)).filter(Boolean) : [1024, 512, 256, 128, 64];
const WORDMARK = (args.wordmark && args.wordmark !== 'true') ? args.wordmark : '';
fs.mkdirSync(OUT, { recursive: true });

// ---------- rasterizer: prefer rsvg-convert, else @resvg/resvg-js ----------
let hasRsvg = false;
try { execSync('command -v rsvg-convert', { stdio: 'ignore' }); hasRsvg = true; } catch {}

let Resvg = null;
function loadResvg() {
  if (Resvg) return Resvg;
  const tries = [
    '@resvg/resvg-js',
    path.join(process.cwd(), 'node_modules/@resvg/resvg-js'),
    '/workspace/.svgtools/node_modules/@resvg/resvg-js',
  ];
  for (const t of tries) { try { Resvg = require(t).Resvg; return Resvg; } catch {} }
  // last resort: install into a tools dir
  const toolsDir = fs.existsSync('/workspace') ? '/workspace/.svgtools' : path.join(process.cwd(), '.svgtools');
  console.error(`installing @resvg/resvg-js into ${toolsDir} (one-time)...`);
  fs.mkdirSync(toolsDir, { recursive: true });
  execSync('npm install --no-audit --no-fund --silent @resvg/resvg-js', { cwd: toolsDir, stdio: 'inherit' });
  Resvg = require(path.join(toolsDir, 'node_modules/@resvg/resvg-js')).Resvg;
  return Resvg;
}

function rasterize(svgPath, width, outPath) {
  if (hasRsvg) {
    execFileSync('rsvg-convert', ['-w', String(width), '-o', outPath, svgPath]); // transparent by default
    return fs.statSync(outPath).size;
  }
  const R = loadResvg();
  const png = new R(fs.readFileSync(svgPath), { fitTo: { mode: 'width', value: width }, font: { loadSystemFonts: true } }).render().asPng();
  fs.writeFileSync(outPath, png);
  return png.length;
}

// ---------- svg helpers ----------
// Generated SVGs often ship a bad viewBox (content cropped or drifted off-center).
// Fix it deterministically: compute the real content bounding box and rewrite the
// viewBox to a square centered on the content with even padding. Path data is
// untouched, so the geometry-identity guarantee holds. Best-effort: if bbox
// computation is unavailable, the original svg is returned unchanged.
function normalizeViewBox(svg, padFrac = 0.08, square = true) {
  try {
    const R = loadResvg();
    const r = new R(svg); // natural render size = viewBox size, so bbox ≈ user units
    const bbox = (typeof r.getBBox === 'function' && r.getBBox()) || (typeof r.innerBBox === 'function' && r.innerBBox());
    if (!bbox || !bbox.width || !bbox.height) return svg;
    const pad = Math.max(bbox.width, bbox.height) * padFrac;
    const w = (square ? Math.max(bbox.width, bbox.height) : bbox.width) + 2 * pad;
    const h = (square ? Math.max(bbox.width, bbox.height) : bbox.height) + 2 * pad;
    const cx = bbox.x + bbox.width / 2, cy = bbox.y + bbox.height / 2;
    const vb = `${(cx - w / 2).toFixed(2)} ${(cy - h / 2).toFixed(2)} ${w.toFixed(2)} ${h.toFixed(2)}`;
    const open = svg.match(/<svg\b[^>]*>/i)[0];
    const newOpen = open
      .replace(/\sviewBox\s*=\s*"[^"]*"/i, '')
      .replace(/\swidth\s*=\s*"[^"]*"/i, '')
      .replace(/\sheight\s*=\s*"[^"]*"/i, '')
      .replace(/<svg\b/i, `<svg viewBox="${vb}"`);
    return svg.replace(open, newOpen);
  } catch {
    return svg;
  }
}

// SVG <style> blocks are DOCUMENT-GLOBAL: when two generated fragments are
// composed into one file, the later stylesheet's classes override the earlier
// one's (e.g. both use .cls-0), silently recoloring or hiding a mark. Flatten
// every class rule to inline fill/stroke attributes and strip the <style> tag
// so each fragment is self-contained.
function flattenStyles(inner) {
  const rules = {};
  inner = inner.replace(/<style\b[^>]*>([\s\S]*?)<\/style>/gi, (_, css) => {
    for (const m of css.matchAll(/\.([\w-]+)\s*\{([^}]*)\}/g)) {
      const decl = {};
      for (const d of m[2].matchAll(/([\w-]+)\s*:\s*([^;]+)/g)) decl[d[1].trim()] = d[2].trim();
      rules[m[1]] = decl;
    }
    return '';
  });
  return inner.replace(/class\s*=\s*"([\w-]+)"/g, (whole, cls) => {
    const decl = rules[cls];
    if (!decl) return whole;
    return Object.entries(decl)
      .filter(([k]) => /^(fill|stroke|stroke-width|fill-rule|opacity|fill-opacity|stroke-linecap|stroke-linejoin)$/.test(k))
      .map(([k, v]) => `${k}="${v}"`).join(' ');
  });
}

const WHITE_RE = /^#(?:fff(?:fff)?|f7f7fa|fefefe)$/i;
// Force every non-white fill to the brand color (whites are preserved as
// negative space). Used by --recolor for brand-exact hue.
function recolorFills(svg, color) {
  return svg.replace(/fill="(#[0-9a-fA-F]{3,6})"/g, (whole, hex) => (WHITE_RE.test(hex) ? whole : `fill="${color}"`));
}

function markParts(markSvg) {
  const open = markSvg.match(/<svg\b[^>]*>/i)[0];
  const vb = (open.match(/viewBox\s*=\s*"([^"]+)"/i) || [])[1] || '0 0 100 100';
  const inner = flattenStyles(markSvg.replace(/^[\s\S]*?<svg\b[^>]*>/i, '').replace(/<\/svg>\s*$/i, ''));
  return { vb, inner };
}
function embed(parts, { x, y, size, mono }) {
  let inner = parts.inner;
  if (mono) inner = inner.replace(/#[0-9a-fA-F]{6}\b/g, mono).replace(/#[0-9a-fA-F]{3}\b/g, mono);
  return `<svg x="${x}" y="${y}" width="${size}" height="${size}" viewBox="${parts.vb}" preserveAspectRatio="xMidYMid meet" overflow="visible">${inner}</svg>`;
}
function doc(w, h, body) {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}">${body}</svg>`;
}
function pathData(svg) { return (svg.match(/\bd\s*=\s*"[^"]+"/g) || []).sort(); }
function esc(s) { return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); }
function darken(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex); if (!m) return '#111111';
  const n = parseInt(m[1], 16);
  const r = Math.round(((n >> 16) & 255) * 0.35), g = Math.round(((n >> 8) & 255) * 0.35), b = Math.round((n & 255) * 0.35);
  return '#' + [r, g, b].map((v) => v.toString(16).padStart(2, '0')).join('');
}

// ---------- build ----------
const rawMarkSvg = fs.readFileSync(args.mark, 'utf8');
if (!/<svg[\s>]/i.test(rawMarkSvg)) die('mark file does not contain an <svg> element');
let markSvg = args['no-crop'] ? rawMarkSvg : normalizeViewBox(rawMarkSvg);
if (markSvg !== rawMarkSvg) console.error('normalized mark viewBox to content bounding box');
{
  // flatten class styles in the stored mark too, so mark.svg is self-contained
  const open = markSvg.match(/<svg\b[^>]*>/i)[0];
  markSvg = open + flattenStyles(markSvg.slice(markSvg.indexOf(open) + open.length));
}
if (args.recolor) {
  markSvg = recolorFills(markSvg, COLOR);
  console.error(`recolored mark fills to ${COLOR} (whites preserved)`);
}
const parts = markParts(markSvg);
const canonical = pathData(markSvg);
if (canonical.length === 0) console.error('warning: mark has no <path d="..."> data; geometry check will be trivial');

// normalize the raw mark into its own clean square document too
const assets = {};
assets['mark.svg'] = markSvg;

if (NAME) {
  const hF = 72, hTextW = Math.ceil([...NAME].length * hF * 0.66);
  assets['lockup-horizontal.svg'] = doc(150 + hTextW + 30, 160,
    embed(parts, { x: 10, y: 20, size: 120 }) +
    `<text x="150" y="98" font-family='${FONT}' font-weight="${WEIGHT}" font-size="${hF}" fill="${COLOR}">${esc(NAME)}</text>`);

  const sF = 54, sW = Math.max(280, Math.ceil([...NAME].length * sF * 0.66) + 40);
  assets['lockup-stacked.svg'] = doc(sW, 260,
    embed(parts, { x: sW / 2 - 65, y: 10, size: 130 }) +
    `<text x="${sW / 2}" y="235" text-anchor="middle" font-family='${FONT}' font-weight="${WEIGHT}" font-size="${sF}" fill="${COLOR}">${esc(NAME)}</text>`);
}

// optional generated wordmark placed beside the mark (alternative to typeset text)
if (WORDMARK && fs.existsSync(WORDMARK)) {
  const wparts = markParts(normalizeViewBox(fs.readFileSync(WORDMARK, 'utf8'), 0.04, false));
  if (args.recolor) wparts.inner = recolorFills(wparts.inner, COLOR);
  // size the wordmark box from its real aspect ratio (height fixed at 80)
  const [, , wvbW, wvbH] = wparts.vb.split(/\s+/).map(Number);
  const wmW = Math.max(120, Math.round(80 * (wvbW / (wvbH || 1))));
  assets['lockup-wordmark.svg'] = doc(150 + wmW + 30, 160,
    embed(parts, { x: 10, y: 20, size: 120 }) +
    `<svg x="150" y="40" width="${wmW}" height="80" viewBox="${wparts.vb}" preserveAspectRatio="xMidYMid meet">${wparts.inner}</svg>`);
}

assets['app-icon.svg'] = doc(512, 512,
  `<rect x="0" y="0" width="512" height="512" rx="112" fill="${COLOR}"/>` +
  embed(parts, { x: 96, y: 96, size: 320, mono: '#FFFFFF' }));

assets['mono-black.svg'] = doc(200, 200, embed(parts, { x: 20, y: 20, size: 160, mono: '#111111' }));
assets['reverse-white.svg'] = doc(200, 200, embed(parts, { x: 20, y: 20, size: 160, mono: '#FFFFFF' }));
assets['favicon.svg'] = doc(64, 64, embed(parts, { x: 4, y: 4, size: 56 }));

// ---------- write SVGs, rasterize, verify ----------
const ladderFor = (file) => {
  if (file === 'favicon.svg') return [16, 32, 48, 64, 128];
  if (file.startsWith('lockup')) return [2000, 1000, 500];
  return SIZES; // square assets
};

const manifest = { name: NAME, color: COLOR, markPaths: canonical.length, assets: [], geometryIdentity: true };
let fail = 0;
for (const [file, svg] of Object.entries(assets)) {
  const svgPath = path.join(OUT, file);
  fs.writeFileSync(svgPath, svg);
  const pngs = [];
  for (const w of ladderFor(file)) {
    const pngPath = path.join(OUT, file.replace('.svg', `-${w}.png`));
    const bytes = rasterize(svgPath, w, pngPath);
    pngs.push({ width: w, file: path.basename(pngPath), bytes });
  }
  const present = pathData(svg);
  const identical = canonical.every((d) => present.includes(d));
  if (!identical) { fail++; manifest.geometryIdentity = false; }
  manifest.assets.push({ svg: file, geometryIdentical: identical, pngs });
  console.log(`${identical ? 'OK ' : 'XX '} ${file}  (${pngs.map((p) => p.width).join('/')})`);
}
const markHash = crypto.createHash('sha256').update(canonical.join('|')).digest('hex').slice(0, 12);
manifest.markHash = markHash;
fs.writeFileSync(path.join(OUT, 'manifest.json'), JSON.stringify(manifest, null, 2));

console.log(`\nmark id ${markHash} · ${canonical.length} path(s) · rasterizer: ${hasRsvg ? 'rsvg-convert' : '@resvg/resvg-js'}`);
console.log(`pack written to ${OUT}/`);
console.log(fail === 0 ? 'GEOMETRY IDENTITY: PASS ✅' : `GEOMETRY IDENTITY: FAIL ❌ (${fail} asset(s))`);
process.exit(fail === 0 ? 0 : 2);
