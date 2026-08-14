# Files, reports, and media

Status: proposed
Build in: phases 2 to 6
Teams: Artifacts, Web, Mobile, Desktop, Agent Runtime

## Why one artifact system

Hivy already has Files, Drive, Canvas, Sheets, brands, and apps. Users shouldn't have to learn six unrelated output systems.

An **artifact** is any result people can review, edit, send, or publish: document, slide deck, workbook, PDF, report, image, audio, video, chart, or internal app.

## What every artifact stores

Each artifact has an org, team, project, work item, producing run, owner, type, file details, classification, region, retention rule, sharing rule, and review state.

Every edit creates an immutable version with a content hash, parent version, source inputs, citations, transformations, model/tool history, checks, and storage references. Users can compare, restore, fork, comment, accept, reject, and download according to access.

An approval always names one version. Sending a file is a separate governed action.

## Creation flow

The agent first proposes the format, sources, and template. Hivy checks source access and output rules, creates the native file and safe preview, then tests structure and rendering.

The user reviews, marks up, requests edits, and accepts. Only then can policy send or publish it. A retry can't create duplicate final versions or send an older file.

## Documents

DOCX support needs headings, lists, tables, notes, citations, headers, footers, page breaks, images, comments, tracked edits, and templates. Editing an uploaded file should preserve its structure.

Check fonts, fields, table clipping, page breaks, links, contrast, and unresolved citations. Export DOCX and PDF. Markdown may be an extra format, not a substitute.

## Presentations

PPTX needs layouts, themes, masters, notes, citations, tables, charts, images, and accessible reading order. Brand kits set fonts, colors, logos, image rules, and templates.

Review should expose each slide, speaker notes, overflow, sources, and locked elements. Export PPTX and PDF.

## Spreadsheets

XLSX needs sheets, formulas, formatting, tables, filters, frozen panes, named ranges, charts, comments, and validation. Keep formulas when editing instead of flattening them into values.

Each generated number links to its source, query, transformation, and cell range. Check formula errors, loops, hidden sheets, mismatched ranges, and totals. Connected refresh creates a new version or reviewed update.

## PDFs

Read text and layout, extract tables, cite page regions, fill supported forms, annotate, combine, split, redact, and create accessible PDFs. OCR keeps confidence and coordinates.

Real redaction removes the hidden text and metadata. Regulated signatures should use a proven signing provider.

## Images, audio, and video

Image work needs source rights, brand rules, crops, masks, version history, and provenance. Apply company rules to faces, trademarks, customer screenshots, and private material.

Audio holds the recording, transcript, speakers, timestamps, summary, decisions, and clips. Recording and transcript may expire at different times.

Video comes later. If customer demand earns it, build storyboards, scripts, scenes, voices, captions, likeness consent, rights checks, review, and export. Don't let it delay office files.

## Charts and apps

Charts keep their data snapshot, query, filters, units, definitions, and sources. Static exports state their filters and creation time.

Generated apps keep source code, build, runtime, routes, rights, environment, audience, scan, and publish history. Preview runs in isolation. Writes inside the app still go through normal policy.

## Review and safety

Comments can attach to text, a page region, slide element, cell range, media time, image region, chart mark, or app control. User edits become real versions; agents can't overwrite accepted work.

Encrypt source files, outputs, previews, and thumbnails. Scan uploads and archives. Apply retention, region, legal hold, keys, and sharing to every copy. Preview workers can't fetch arbitrary internal URLs.

## Requirements

| ID | Hivy must |
|---|---|
| **ART-001** | Put every output in one versioned artifact model. |
| **ART-002** | Keep source, citation, transformation, and run history. |
| **ART-003** | Create native DOCX, PPTX, XLSX, and PDF with previews. |
| **ART-004** | Preserve structure and formulas during edits. |
| **ART-005** | Run file-specific structure, visual, formula, link, and access checks. |
| **ART-006** | Support anchored comments, compare, restore, fork, accept, and reject. |
| **ART-007** | Tie approvals and delivery to exact versions. |
| **ART-008** | Apply retention, region, labels, keys, and sharing to every copy. |
| **ART-009** | Keep chart data and definition history. |
| **ART-010** | Isolate and scan internal apps before publish. |
| **ART-011** | Let raw media and transcripts use different retention. |
| **ART-012** | Provide accessible previews on web, desktop, and mobile. |

## Done when

- Every version links back to its job, inputs, agent, and checks.
- An approved version can never change in place.
- Office files open in standard apps and match the reviewed preview within set limits.
- Spreadsheet formulas survive export and edit.
- PDF redaction removes selection, extraction, layers, and metadata.
- Apps can't call ungranted data or actions.
- Revoked links stop working at once.

Measure creation and preview success, time to acceptance, revision count, formula and citation defects, delivery success, revocation time, security events, and format use.
