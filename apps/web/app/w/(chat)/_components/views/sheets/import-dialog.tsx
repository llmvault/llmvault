"use client"

import { useEffect, useMemo, useState } from "react"
import { Button, Modal, ProgressBar, Spinner, Switch, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import Papa from "papaparse"
import { useDropzone } from "react-dropzone"
import { extractErrorMessage } from "@/lib/api/error"
import {
  createImport,
  fetchImportJob,
  sheetKeys,
  uploadSheetObject,
  type SheetField,
} from "@/app/w/(chat)/_lib/sheets"

const PREVIEW_ROWS = 20

interface CsvPreview {
  rows: string[][]
  delimiter: string
}

type ImportPhase = "configure" | "starting" | "running"

export function ImportDialog({
  isOpen,
  onOpenChange,
  sheetId,
  pageId,
  fields,
}: {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  sheetId: string
  pageId: string
  fields: SheetField[]
}) {
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<CsvPreview | null>(null)
  const [hasHeader, setHasHeader] = useState(true)
  const [createFieldsMode, setCreateFieldsMode] = useState(true)
  const [mapping, setMapping] = useState<Record<string, string>>({})
  const [phase, setPhase] = useState<ImportPhase>("configure")
  const [jobId, setJobId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const reset = () => {
    setFile(null)
    setPreview(null)
    setHasHeader(true)
    setCreateFieldsMode(true)
    setMapping({})
    setPhase("configure")
    setJobId(null)
    setError(null)
  }

  const close = (open: boolean) => {
    if (!open) reset()
    onOpenChange(open)
  }

  const dropzone = useDropzone({
    accept: { "text/csv": [".csv"] },
    multiple: false,
    onDrop: (accepted) => {
      const dropped = accepted[0]
      if (!dropped) return
      setError(null)
      setFile(dropped)
      Papa.parse<string[]>(dropped, {
        preview: PREVIEW_ROWS + 1,
        skipEmptyLines: true,
        complete: (result) => {
          setPreview({
            rows: result.data,
            delimiter: result.meta.delimiter || ",",
          })
        },
        error: (parseError) => {
          setError(parseError.message || "Could not parse the CSV file.")
        },
      })
    },
  })

  const headerRow = useMemo(() => {
    const first = preview?.rows[0] ?? []
    if (hasHeader) return first
    return first.map((_, index) => `Column ${index + 1}`)
  }, [preview, hasHeader])
  const bodyRows = useMemo(() => {
    const rows = preview?.rows ?? []
    return hasHeader ? rows.slice(1, PREVIEW_ROWS + 1) : rows.slice(0, PREVIEW_ROWS)
  }, [preview, hasHeader])

  const jobQuery = useQuery({
    enabled: Boolean(jobId),
    queryKey: sheetKeys.importJob(jobId ?? ""),
    queryFn: ({ signal }) => fetchImportJob(jobId ?? "", signal),
    // SSE import_progress events land in this cache too; polling is the
    // fallback when the stream is down.
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === "completed" || status === "failed" ? false : 2000
    },
  })
  const job = jobQuery.data
  const jobStatus = job?.status

  useEffect(() => {
    if (jobStatus === "completed") {
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.rowsPrefix(pageId),
      })
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.structure(sheetId),
      })
      toast.success("CSV import completed")
    }
  }, [jobStatus, pageId, queryClient, sheetId])

  const startImport = async () => {
    if (!file) return
    setPhase("starting")
    setError(null)
    try {
      const objectKey = await uploadSheetObject("sheet_import", file)
      const options: Record<string, unknown> = {
        has_header: hasHeader,
        ...(preview?.delimiter && preview.delimiter !== ","
          ? { delimiter: preview.delimiter }
          : {}),
      }
      if (createFieldsMode) {
        options.create_fields = true
      } else {
        options.create_fields = false
        const fieldMapping: Record<string, string> = {}
        for (const [column, fieldId] of Object.entries(mapping)) {
          if (fieldId) fieldMapping[column] = fieldId
        }
        options.field_mapping = fieldMapping
      }
      const created = await createImport(sheetId, pageId, objectKey, options)
      if (!created.id) throw new Error("Import job was not created")
      setJobId(created.id)
      setPhase("running")
    } catch (startError) {
      setError(extractErrorMessage(startError, "Could not start the import"))
      setPhase("configure")
    }
  }

  const progressValue =
    job && job.total_rows
      ? Math.min(100, ((job.processed_rows ?? 0) / job.total_rows) * 100)
      : undefined

  return (
    <Modal isOpen={isOpen} onOpenChange={close}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="lg">
          <Modal.Dialog className="p-6">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>Import CSV</Modal.Heading>
            </Modal.Header>

            {phase === "running" ? (
              <div className="flex flex-col gap-3 py-4">
                {jobStatus === "failed" ? (
                  <div className="flex items-start gap-2 rounded-lg border border-border bg-default p-3">
                    <Icon
                      icon="lucide:circle-alert"
                      className="mt-0.5 h-4 w-4 shrink-0 text-danger"
                    />
                    <p className="text-sm text-foreground">
                      {job?.error || "The import failed."}
                    </p>
                  </div>
                ) : jobStatus === "completed" ? (
                  <div className="flex items-center gap-2">
                    <Icon
                      icon="lucide:circle-check"
                      className="h-4 w-4 text-success"
                    />
                    <p className="text-sm text-foreground">
                      Imported {job?.processed_rows ?? 0} rows.
                    </p>
                  </div>
                ) : (
                  <>
                    <div className="flex items-center gap-2">
                      <Spinner size="sm" />
                      <p className="text-sm text-foreground">
                        Importing
                        {job?.total_rows
                          ? ` ${job.processed_rows ?? 0} of ${job.total_rows} rows…`
                          : `… ${job?.processed_rows ?? 0} rows processed`}
                      </p>
                    </div>
                    <ProgressBar
                      aria-label="Import progress"
                      value={progressValue}
                      isIndeterminate={progressValue === undefined}
                    />
                  </>
                )}
                <Modal.Footer className="px-0">
                  <Button variant="secondary" onPress={() => close(false)}>
                    {jobStatus === "completed" || jobStatus === "failed"
                      ? "Close"
                      : "Run in background"}
                  </Button>
                </Modal.Footer>
              </div>
            ) : (
              <div className="flex flex-col gap-3 py-2">
                {!file ? (
                  <div
                    {...dropzone.getRootProps()}
                    className={`flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed px-4 py-10 transition-colors ${
                      dropzone.isDragActive
                        ? "border-accent bg-accent/10"
                        : "border-border hover:bg-default"
                    }`}
                  >
                    <input {...dropzone.getInputProps()} />
                    <Icon
                      icon="lucide:file-up"
                      className="h-6 w-6 text-muted"
                    />
                    <p className="text-sm text-foreground">
                      Drop a CSV file here, or click to browse
                    </p>
                    <p className="text-xs text-muted">Up to 50 MB</p>
                  </div>
                ) : (
                  <>
                    <div className="flex items-center gap-2 rounded-lg border border-border px-3 py-2">
                      <Icon
                        icon="lucide:file-text"
                        className="h-4 w-4 shrink-0 text-muted"
                      />
                      <span className="min-w-0 flex-1 truncate text-sm text-foreground">
                        {file.name}
                      </span>
                      <Button
                        variant="ghost"
                        size="sm"
                        isIconOnly
                        aria-label="Remove file"
                        onPress={() => {
                          setFile(null)
                          setPreview(null)
                          setMapping({})
                        }}
                      >
                        <Icon icon="lucide:x" className="h-3.5 w-3.5" />
                      </Button>
                    </div>

                    <div className="flex flex-wrap items-center gap-4">
                      <Switch
                        isSelected={hasHeader}
                        onChange={setHasHeader}
                      >
                        First row is a header
                      </Switch>
                      <Switch
                        isSelected={createFieldsMode}
                        onChange={setCreateFieldsMode}
                      >
                        Create columns automatically
                      </Switch>
                      {preview ? (
                        <span className="text-xs text-muted">
                          Delimiter:{" "}
                          {preview.delimiter === "\t"
                            ? "tab"
                            : preview.delimiter}
                        </span>
                      ) : null}
                    </div>

                    {!createFieldsMode ? (
                      <div className="flex flex-col gap-1.5 rounded-lg border border-border p-2">
                        <p className="text-xs text-muted">
                          Map CSV columns to existing columns (unmapped columns
                          are skipped).
                        </p>
                        {headerRow.map((column, index) => {
                          const key = hasHeader ? column : String(index)
                          return (
                            <div
                              key={`${index}-${column}`}
                              className="flex items-center gap-2"
                            >
                              <span className="w-40 truncate text-xs text-foreground">
                                {column}
                              </span>
                              <Icon
                                icon="lucide:arrow-right"
                                className="h-3 w-3 shrink-0 text-muted"
                              />
                              <select
                                aria-label={`Map ${column}`}
                                value={mapping[key] ?? ""}
                                onChange={(event) =>
                                  setMapping((prev) => ({
                                    ...prev,
                                    [key]: event.target.value,
                                  }))
                                }
                                className="h-7 min-w-0 flex-1 rounded-lg border border-border bg-surface px-2 text-xs text-foreground outline-none focus:border-accent"
                              >
                                <option value="">Skip column</option>
                                {fields
                                  .filter((field) => field.id)
                                  .map((field) => (
                                    <option
                                      key={field.id}
                                      value={field.id ?? ""}
                                    >
                                      {field.name}
                                    </option>
                                  ))}
                              </select>
                            </div>
                          )
                        })}
                      </div>
                    ) : null}

                    {preview && bodyRows.length > 0 ? (
                      <div className="max-h-48 overflow-auto rounded-lg border border-border">
                        <table className="w-full text-left text-xs">
                          <thead>
                            <tr className="border-b border-border bg-default">
                              {headerRow.map((column, index) => (
                                <th
                                  key={`${index}-${column}`}
                                  className="px-2 py-1.5 font-medium whitespace-nowrap text-muted"
                                >
                                  {column}
                                </th>
                              ))}
                            </tr>
                          </thead>
                          <tbody>
                            {bodyRows.map((row, rowIndex) => (
                              <tr
                                key={rowIndex}
                                className="border-b border-border last:border-b-0"
                              >
                                {headerRow.map((_, colIndex) => (
                                  <td
                                    key={colIndex}
                                    className="max-w-48 truncate px-2 py-1 whitespace-nowrap text-foreground"
                                  >
                                    {row[colIndex] ?? ""}
                                  </td>
                                ))}
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    ) : null}
                  </>
                )}

                {error ? (
                  <div className="flex items-start gap-2 rounded-lg border border-border bg-default p-3">
                    <Icon
                      icon="lucide:circle-alert"
                      className="mt-0.5 h-4 w-4 shrink-0 text-danger"
                    />
                    <p className="text-sm text-foreground">{error}</p>
                  </div>
                ) : null}

                <Modal.Footer className="px-0">
                  <Button variant="secondary" onPress={() => close(false)}>
                    Cancel
                  </Button>
                  <Button
                    isDisabled={!file || phase === "starting"}
                    onPress={() => void startImport()}
                  >
                    {phase === "starting" ? <Spinner size="sm" /> : null}
                    Start import
                  </Button>
                </Modal.Footer>
              </div>
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
