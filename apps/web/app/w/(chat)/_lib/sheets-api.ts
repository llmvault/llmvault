import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import { newMutationId } from "@/app/w/(chat)/_lib/sheets-mutation-id"
import {
  ROWS_PAGE_SIZE,
  type RowsQueryInput,
  type SheetFieldSpec,
} from "@/app/w/(chat)/_lib/sheets-types"

/* ------------------------------------------------------------------ */
/* API calls (typed openapi-fetch client through /api/proxy)           */
/* ------------------------------------------------------------------ */

async function unwrap<T>(
  result: Promise<{ data?: T; error?: unknown }>
): Promise<T> {
  const { data, error } = await result
  if (error !== undefined) {
    throw new Error(extractErrorMessage(error, "Sheets request failed"))
  }
  return data as T
}

export function fetchSheets(channelId: string, signal?: AbortSignal) {
  return unwrap(
    api.GET("/v1/sheets", {
      params: { query: { channel_id: channelId, limit: 200 } },
      signal,
    })
  )
}

export function createSheet(channelId: string, name: string) {
  return unwrap(
    api.POST("/v1/sheets", { body: { channel_id: channelId, name } })
  )
}

export function fetchSheetStructure(sheetID: string, signal?: AbortSignal) {
  return unwrap(
    api.GET("/v1/sheets/{sheetID}", { params: { path: { sheetID } }, signal })
  )
}

export function createSheetPage(sheetID: string, name: string) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages", {
      params: { path: { sheetID } },
      body: { name, mutation_id: newMutationId() },
    })
  )
}

export function updateSheetPage(
  sheetID: string,
  pageID: string,
  body: { name?: string; position?: number; display_field_id?: string }
) {
  return unwrap(
    api.PATCH("/v1/sheets/{sheetID}/pages/{pageID}", {
      params: { path: { sheetID, pageID } },
      body: { ...body, mutation_id: newMutationId() },
    })
  )
}

export function deleteSheetPage(sheetID: string, pageID: string) {
  return unwrap(
    api.DELETE("/v1/sheets/{sheetID}/pages/{pageID}", {
      params: { path: { sheetID, pageID } },
    })
  )
}

export function createSheetField(
  sheetID: string,
  pageID: string,
  spec: SheetFieldSpec
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/fields", {
      params: { path: { sheetID, pageID } },
      body: { ...spec, mutation_id: newMutationId() },
    })
  )
}

export function updateSheetField(
  sheetID: string,
  pageID: string,
  fieldID: string,
  body: {
    name?: string
    type?: string
    options?: Record<string, unknown>
    position?: number
  }
) {
  return unwrap(
    api.PATCH("/v1/sheets/{sheetID}/pages/{pageID}/fields/{fieldID}", {
      params: { path: { sheetID, pageID, fieldID } },
      body: { ...body, mutation_id: newMutationId() },
    })
  )
}

export function deleteSheetField(
  sheetID: string,
  pageID: string,
  fieldID: string
) {
  return unwrap(
    api.DELETE("/v1/sheets/{sheetID}/pages/{pageID}/fields/{fieldID}", {
      params: { path: { sheetID, pageID, fieldID } },
    })
  )
}

export function queryRows(
  sheetID: string,
  pageID: string,
  input: RowsQueryInput,
  signal?: AbortSignal
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/rows/query", {
      params: { path: { sheetID, pageID } },
      body: {
        filter: input.filter,
        search: input.search || undefined,
        sorts: input.sorts?.length ? input.sorts : undefined,
        cursor: input.cursor || undefined,
        limit: input.limit ?? ROWS_PAGE_SIZE,
        resolve_relations: input.resolve_relations ?? true,
      },
      signal,
    })
  )
}

export function insertRows(
  sheetID: string,
  pageID: string,
  rows: { data: Record<string, unknown>; position?: number }[],
  mutationId = newMutationId()
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/rows", {
      params: { path: { sheetID, pageID } },
      body: { rows, mutation_id: mutationId },
    })
  )
}

export function updateRows(
  sheetID: string,
  pageID: string,
  rows: { id: string; data?: Record<string, unknown>; position?: number }[],
  mutationId = newMutationId()
) {
  return unwrap(
    api.PATCH("/v1/sheets/{sheetID}/pages/{pageID}/rows", {
      params: { path: { sheetID, pageID } },
      body: { rows, mutation_id: mutationId },
    })
  )
}

export function deleteRows(
  sheetID: string,
  pageID: string,
  ids: string[],
  mutationId = newMutationId()
) {
  return unwrap(
    api.DELETE("/v1/sheets/{sheetID}/pages/{pageID}/rows", {
      params: { path: { sheetID, pageID } },
      body: { ids, mutation_id: mutationId },
    })
  )
}

export function fetchViews(
  sheetID: string,
  pageID: string,
  signal?: AbortSignal
) {
  return unwrap(
    api.GET("/v1/sheets/{sheetID}/pages/{pageID}/views", {
      params: { path: { sheetID, pageID } },
      signal,
    })
  )
}

export function createView(
  sheetID: string,
  pageID: string,
  body: { name: string; type?: string; config?: Record<string, unknown> }
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/views", {
      params: { path: { sheetID, pageID } },
      body,
    })
  )
}

export function updateView(
  sheetID: string,
  pageID: string,
  viewID: string,
  body: { config?: Record<string, unknown>; name?: string }
) {
  return unwrap(
    api.PATCH("/v1/sheets/{sheetID}/pages/{pageID}/views/{viewID}", {
      params: { path: { sheetID, pageID, viewID } },
      body,
    })
  )
}

export function deleteView(sheetID: string, pageID: string, viewID: string) {
  return unwrap(
    api.DELETE("/v1/sheets/{sheetID}/pages/{pageID}/views/{viewID}", {
      params: { path: { sheetID, pageID, viewID } },
    })
  )
}

export function fetchOperations(
  sheetID: string,
  pageID: string,
  signal?: AbortSignal
) {
  return unwrap(
    api.GET("/v1/sheets/{sheetID}/pages/{pageID}/operations", {
      params: { path: { sheetID, pageID }, query: { limit: 25 } },
      signal,
    })
  )
}

export function revertOperation(
  sheetID: string,
  pageID: string,
  operationID: string
) {
  return unwrap(
    api.POST(
      "/v1/sheets/{sheetID}/pages/{pageID}/operations/{operationID}/revert",
      {
        params: { path: { sheetID, pageID, operationID } },
      }
    )
  )
}

export function createImport(
  sheetID: string,
  pageID: string,
  objectKey: string,
  options: Record<string, unknown>
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/imports", {
      params: { path: { sheetID, pageID } },
      body: {
        object_key: objectKey,
        options,
        mutation_id: newMutationId(),
      },
    })
  )
}

export function fetchImportJob(jobID: string, signal?: AbortSignal) {
  return unwrap(
    api.GET("/v1/sheets/imports/{jobID}", {
      params: { path: { jobID } },
      signal,
    })
  )
}

export function fetchAttachmentDownloadURLs(
  sheetID: string,
  pageID: string,
  keys: string[]
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/attachments/download-url", {
      params: { path: { sheetID, pageID } },
      body: { keys },
    })
  )
}

export function mintLiveToken(sheetID: string) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/live-token", {
      params: { path: { sheetID } },
    })
  )
}

export function exportCsvUrl(sheetID: string, pageID: string): string {
  return `/api/proxy/v1/sheets/${encodeURIComponent(
    sheetID
  )}/pages/${encodeURIComponent(pageID)}/export.csv`
}

/** Signs an upload for a sheets asset type and PUTs the file to storage. */
export async function uploadSheetObject(
  assetType: "sheet_import" | "sheet_attachment",
  file: File
): Promise<string> {
  const contentType = file.type || "application/octet-stream"
  const signed = await unwrap(
    api.POST("/v1/uploads/sign", {
      body: {
        asset_type: assetType,
        content_type: contentType,
        filename: file.name,
        size_bytes: file.size,
      },
    })
  )
  if (!signed.upload_url || !signed.key) {
    throw new Error("Upload could not be signed")
  }
  // eslint-disable-next-line no-restricted-globals -- signed-storage upload: PUT straight to the pre-signed object-store URL, not the Hivy API.
  const putRes = await fetch(signed.upload_url, {
    method: signed.upload_method || "PUT",
    headers: {
      "Content-Type": contentType,
      ...(signed.required_headers ?? {}),
    },
    body: file,
  })
  if (!putRes.ok) throw new Error(`Upload failed with HTTP ${putRes.status}`)
  return signed.key
}
