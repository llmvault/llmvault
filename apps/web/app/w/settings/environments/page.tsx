"use client"

import { type FormEvent, useEffect, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"

const DEFAULT_PORTS = [3000, 5173, 8000, 8080]
const RESERVED_RUNTIME_PORT = 7080
const MAX_PORTS = 20

export default function EnvironmentsSettingsPage() {
  const queryClient = useQueryClient()
  const orgQuery = $api.useQuery("get", "/v1/orgs/current")
  const updateOrg = $api.useMutation("patch", "/v1/orgs/current")
  const savedPorts = orgQuery.data?.sandbox_exposed_ports ?? DEFAULT_PORTS
  const savedInput = formatPorts(savedPorts)
  const [portsInput, setPortsInput] = useState(savedInput)

  useEffect(() => {
    setPortsInput(savedInput)
  }, [savedInput])

  const parsed = useMemo(() => parsePorts(portsInput), [portsInput])
  const hasChanges = parsed.value !== savedInput
  const canSave =
    !orgQuery.isLoading && !updateOrg.isPending && !parsed.error && hasChanges

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSave) return
    updateOrg.mutate(
      { body: { sandbox_exposed_ports: parsed.ports } },
      {
        onSuccess: (org) => {
          setPortsInput(formatPorts(org.sandbox_exposed_ports ?? parsed.ports))
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/orgs/current"],
          })
          toast.success("Preview ports updated")
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update preview ports")
          ),
      }
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold text-foreground">Environments</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Configure workspace sandbox behavior.
        </p>
      </div>

      <section className="bg-surface overflow-hidden rounded-2xl border border-border">
        <div className="flex items-start gap-3 border-b border-border px-4 py-4">
          <div className="bg-default flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground">
            <Icon icon="lucide:radio-tower" className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <h2 className="text-sm font-medium text-foreground">
              Sandbox previews
            </h2>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-muted-foreground">
              These ports are exposed on public preview URLs when a new sandbox
              is created. Existing sandboxes keep the ports they were created
              with until they are recreated.
            </p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4 px-4 py-4">
          <label className="flex flex-col gap-2">
            <span className="text-sm font-medium text-foreground">
              Preview ports
            </span>
            <Input
              value={portsInput}
              onChange={(event) => setPortsInput(event.target.value)}
              placeholder="3000, 5173, 8000, 8080"
              className={cn(
                "w-full max-w-xl",
                parsed.error && "border-danger/70"
              )}
              aria-invalid={Boolean(parsed.error) || undefined}
              aria-describedby="sandbox-preview-ports-help"
            />
          </label>

          <div
            id="sandbox-preview-ports-help"
            className="flex max-w-2xl flex-col gap-2 text-sm leading-5"
          >
            {parsed.error ? (
              <p className="text-danger">{parsed.error}</p>
            ) : (
              <p className="text-muted-foreground">
                Runtime control uses reserved port {RESERVED_RUNTIME_PORT}; Hivy
                adds it automatically.
              </p>
            )}
            <div className="flex flex-wrap gap-1.5">
              {parsed.ports.map((port) => (
                <span
                  key={port}
                  className="rounded-md border border-border bg-card px-2 py-1 font-mono text-xs text-foreground"
                >
                  {port}
                </span>
              ))}
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
            <Button
              type="button"
              variant="tertiary"
              size="sm"
              isDisabled={updateOrg.isPending || !hasChanges}
              onPress={() => setPortsInput(savedInput)}
            >
              Reset
            </Button>
            <Button
              type="submit"
              variant="primary"
              size="sm"
              isDisabled={!canSave}
            >
              {updateOrg.isPending ? (
                <Spinner color="current" size="sm" />
              ) : null}
              Save
            </Button>
          </div>
        </form>
      </section>
    </div>
  )
}

function parsePorts(input: string): {
  ports: number[]
  value: string
  error: string
} {
  const tokens = input
    .split(/[,\s]+/)
    .map((token) => token.trim())
    .filter(Boolean)

  if (tokens.length === 0) {
    return { ports: [], value: "", error: "Add at least one preview port." }
  }

  const seen = new Set<number>()
  const ports: number[] = []
  for (const token of tokens) {
    if (!/^\d+$/.test(token)) {
      return { ports: [], value: "", error: "Use whole-number ports only." }
    }
    const port = Number(token)
    if (port <= 0 || port > 65535) {
      return {
        ports: [],
        value: "",
        error: "Ports must be between 1 and 65535.",
      }
    }
    if (port === RESERVED_RUNTIME_PORT) {
      return {
        ports: [],
        value: "",
        error: `Port ${RESERVED_RUNTIME_PORT} is reserved for Hivy runtime.`,
      }
    }
    if (!seen.has(port)) {
      seen.add(port)
      ports.push(port)
    }
  }

  if (ports.length > MAX_PORTS) {
    return {
      ports: [],
      value: "",
      error: `Use ${MAX_PORTS} or fewer preview ports.`,
    }
  }

  ports.sort((a, b) => a - b)
  return { ports, value: formatPorts(ports), error: "" }
}

function formatPorts(ports: number[]) {
  return [...ports].sort((a, b) => a - b).join(", ")
}
