"use client"

import {
  GridCellKind,
  GridColumnIcon,
  type EditableGridCell,
  type GridCell,
} from "@glideapps/glide-data-grid"
import {
  stringArrayValue,
  type SheetField,
  type SheetRelationRef,
} from "@/app/w/(chat)/_lib/sheets"

/** Field types whose editing happens in our own overlay, not Glide's. */
const CUSTOM_EDITOR_TYPES = new Set([
  "select",
  "multi_select",
  "attachment",
  "relation",
  "date",
  "number",
])

export function usesCustomEditor(type: string | undefined): boolean {
  return CUSTOM_EDITOR_TYPES.has(type ?? "")
}

/** Iconify icon name per field type (used in toolbar pickers). */
export function fieldTypeIcon(type: string | undefined): string {
  switch (type) {
    case "long_text":
      return "lucide:text"
    case "number":
      return "lucide:hash"
    case "checkbox":
      return "lucide:square-check"
    case "select":
      return "lucide:circle-chevron-down"
    case "multi_select":
      return "lucide:list-checks"
    case "date":
      return "lucide:calendar"
    case "url":
      return "lucide:link"
    case "email":
      return "lucide:mail"
    case "phone":
      return "lucide:phone"
    case "attachment":
      return "lucide:paperclip"
    case "relation":
      return "lucide:git-fork"
    default:
      return "lucide:type"
  }
}

/** Glide built-in header icon per field type. */
export function fieldHeaderIcon(type: string | undefined): GridColumnIcon {
  switch (type) {
    case "number":
      return GridColumnIcon.HeaderNumber
    case "checkbox":
      return GridColumnIcon.HeaderBoolean
    case "select":
      return GridColumnIcon.HeaderSingleValue
    case "multi_select":
      return GridColumnIcon.HeaderArray
    case "date":
      return GridColumnIcon.HeaderDate
    case "url":
      return GridColumnIcon.HeaderUri
    case "email":
      return GridColumnIcon.HeaderEmail
    case "phone":
      return GridColumnIcon.HeaderPhone
    case "attachment":
      return GridColumnIcon.HeaderImage
    case "relation":
      return GridColumnIcon.HeaderReference
    case "long_text":
      return GridColumnIcon.HeaderMarkdown
    default:
      return GridColumnIcon.HeaderString
  }
}

const IMAGE_EXTENSIONS = new Set([
  "png",
  "jpg",
  "jpeg",
  "gif",
  "webp",
  "avif",
  "bmp",
  "svg",
])

/** Heuristic: attachment keys keep their filename, so sniff the extension. */
export function isImageAttachmentKey(key: string): boolean {
  const dot = key.lastIndexOf(".")
  if (dot < 0) return false
  return IMAGE_EXTENSIONS.has(key.slice(dot + 1).toLowerCase())
}

export function attachmentFileName(key: string): string {
  const base = key.split("/").pop() ?? key
  // Strip a leading random prefix like "a1b2c3d4-" when present.
  const match = /^[0-9a-f-]{8,}-(.+)$/i.exec(base)
  return match?.[1] ?? base
}

function textCell(value: string, readonly = false): GridCell {
  return {
    kind: GridCellKind.Text,
    data: value,
    displayData: value,
    allowOverlay: !readonly,
  }
}

function bubbleCell(values: string[]): GridCell {
  return {
    kind: GridCellKind.Bubble,
    data: values,
    allowOverlay: false,
  }
}

/**
 * Maps a stored cell value to a Glide display cell for its field type.
 * `imageUrls` maps attachment keys to presigned URLs; attachment cells whose
 * keys are all images with resolved URLs render as thumbnails.
 */
export function cellForValue(
  field: SheetField,
  value: unknown,
  relations: Record<string, SheetRelationRef>,
  imageUrls?: Record<string, string>
): GridCell {
  switch (field.type) {
    case "number": {
      const num = typeof value === "number" ? value : undefined
      // Edited through the custom NumberField overlay, not Glide's.
      return {
        kind: GridCellKind.Number,
        data: num,
        displayData: num === undefined ? "" : String(num),
        allowOverlay: false,
      }
    }
    case "date": {
      // ISO "YYYY-MM-DD"; edited through the custom DatePicker overlay.
      const iso = typeof value === "string" ? value : ""
      return textCell(iso, true)
    }
    case "checkbox":
      return {
        kind: GridCellKind.Boolean,
        data: value === true,
        allowOverlay: false,
      }
    case "url": {
      const url = typeof value === "string" ? value : ""
      return {
        kind: GridCellKind.Uri,
        data: url,
        allowOverlay: true,
        hoverEffect: true,
      }
    }
    case "select": {
      const selected = typeof value === "string" && value ? [value] : []
      return bubbleCell(selected)
    }
    case "multi_select":
      return bubbleCell(stringArrayValue(value))
    case "attachment": {
      const keys = stringArrayValue(value)
      const urls = keys.map((key) =>
        isImageAttachmentKey(key) ? (imageUrls?.[key] ?? "") : ""
      )
      if (keys.length > 0 && urls.every((url) => url !== "")) {
        return {
          kind: GridCellKind.Image,
          data: urls,
          allowOverlay: false,
          rounding: 4,
        }
      }
      return bubbleCell(keys.map(attachmentFileName))
    }
    case "relation":
      return bubbleCell(
        stringArrayValue(value).map(
          (id) => relations[id]?.label || "Untitled row"
        )
      )
    case "long_text": {
      const text = typeof value === "string" ? value : ""
      return {
        kind: GridCellKind.Text,
        data: text,
        displayData: text,
        allowOverlay: true,
        allowWrapping: true,
      }
    }
    default: {
      // text, email, phone — plain text display + overlay editing.
      const text =
        typeof value === "string"
          ? value
          : value === null || value === undefined
            ? ""
            : String(value)
      return textCell(text)
    }
  }
}

/**
 * Converts an edited Glide cell back to the stored value for the field.
 * Returns `undefined` when the edit is not applicable to the field type.
 */
export function valueFromEditedCell(
  field: SheetField,
  cell: EditableGridCell
): { value: unknown } | undefined {
  switch (cell.kind) {
    case GridCellKind.Text: {
      const text = cell.data ?? ""
      return { value: text === "" ? null : text }
    }
    case GridCellKind.Number:
      return { value: cell.data ?? null }
    case GridCellKind.Boolean:
      if (field.type !== "checkbox") return undefined
      return { value: cell.data === true }
    case GridCellKind.Uri: {
      const uri = cell.data ?? ""
      return { value: uri === "" ? null : uri }
    }
    default:
      return undefined
  }
}
