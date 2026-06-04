"use client"

import { useState, useMemo } from "react"
import { Calendar01Icon, ArrowDown01Icon } from "@hugeicons/core-free-icons"
import { HugeiconsIcon } from "@hugeicons/react"
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"

function formatDateLabel(d: string): string {
  const date = new Date(d + "T00:00:00")
  return date.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })
}

function toDateString(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${y}-${m}-${day}`
}

const PRESETS = [
  { label: "7d", days: 7 },
  { label: "30d", days: 30 },
  { label: "90d", days: 90 },
] as const

export function DateRangePicker({
  startDate,
  endDate,
  onChange,
}: {
  startDate: string
  endDate: string
  onChange: (start: string, end: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [draftStart, setDraftStart] = useState(startDate)
  const [draftEnd, setDraftEnd] = useState(endDate)
  const [activeField, setActiveField] = useState<"start" | "end">("start")

  const dateDiff = useMemo(() => {
    const sa = new Date(startDate + "T00:00:00")
    const sb = new Date(endDate + "T00:00:00")
    return Math.max(1, Math.round((sb.getTime() - sa.getTime()) / 86400000) + 1)
  }, [startDate, endDate])

  const rangeLabel = useMemo(() => {
    const preset = PRESETS.find((p) => p.days === dateDiff)
    if (preset) return `Last ${preset.label}`
    return `${formatDateLabel(startDate)} – ${formatDateLabel(endDate)}`
  }, [startDate, endDate, dateDiff])

  function handleDaySelect(dateStr: string) {
    if (activeField === "start") {
      setDraftStart(dateStr)
      if (draftEnd && dateStr > draftEnd) {
        setDraftEnd(dateStr)
      }
      setActiveField("end")
    } else {
      setDraftEnd(dateStr)
      if (draftStart && dateStr < draftStart) {
        setDraftStart(dateStr)
      }
      setActiveField("start")
    }
  }

  function applyPreset(days: number) {
    const end = new Date()
    const start = new Date(end)
    start.setDate(start.getDate() - days)
    setDraftStart(toDateString(start))
    setDraftEnd(toDateString(end))
  }

  function isPresetActive(days: number): boolean {
    const end = toDateString(new Date())
    const start = toDateString(new Date(Date.now() - days * 86400000))
    return draftStart === start && draftEnd === end
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger className="inline-flex h-9 cursor-pointer select-none items-center gap-1.5 rounded-3xl border border-transparent bg-input/50 px-2.5 text-sm font-medium text-foreground whitespace-nowrap transition-[color,box-shadow,background-color] focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30">
        <HugeiconsIcon icon={Calendar01Icon} className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="line-clamp-1">{rangeLabel}</span>
        <HugeiconsIcon
          icon={ArrowDown01Icon}
          strokeWidth={2}
          className="size-3.5 shrink-0 text-muted-foreground"
        />
      </PopoverTrigger>
      <PopoverContent
        className="w-64 rounded-3xl bg-popover/70 p-3 ring-1 ring-foreground/5 backdrop-blur-2xl backdrop-saturate-150 dark:ring-foreground/10"
        align="end"
      >
        <div className="flex flex-col gap-3">
          {/* Date pills */}
          <div className="flex gap-2">
            <button
              onClick={() => setActiveField("start")}
              className={`flex flex-1 flex-col items-center gap-0.5 rounded-full px-3 py-1.5 text-[11px] transition-colors ${
                activeField === "start"
                  ? "bg-foreground text-background"
                  : "border border-border bg-input/50 text-muted-foreground hover:text-foreground"
              }`}
            >
              <span>Start</span>
              <span className="text-xs font-medium">
                {draftStart ? formatDateLabel(draftStart) : "—"}
              </span>
            </button>
            <button
              onClick={() => setActiveField("end")}
              className={`flex flex-1 flex-col items-center gap-0.5 rounded-full px-3 py-1.5 text-[11px] transition-colors ${
                activeField === "end"
                  ? "bg-foreground text-background"
                  : "border border-border bg-input/50 text-muted-foreground hover:text-foreground"
              }`}
            >
              <span>End</span>
              <span className="text-xs font-medium">
                {draftEnd ? formatDateLabel(draftEnd) : "—"}
              </span>
            </button>
          </div>

          {/* Calendar */}
          <Calendar
            startDate={draftStart}
            endDate={draftEnd}
            onSelect={handleDaySelect}
          />

          {/* Presets */}
          <div className="flex gap-2">
            {PRESETS.map((p) => {
              const active = isPresetActive(p.days)
              return (
                <button
                  key={p.label}
                  onClick={() => applyPreset(p.days)}
                  className={`h-7 flex-1 rounded-full text-[11px] font-medium transition-colors ${
                    active
                      ? "bg-foreground text-background"
                      : "border border-border bg-input/50 text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {p.label}
                </button>
              )
            })}
          </div>

          {/* Footer */}
          <div className="flex gap-2">
            <Button
              variant="ghost"
              className="h-8 flex-1 rounded-full text-xs"
              onClick={() => setOpen(false)}
            >
              Cancel
            </Button>
            <Button
              className="h-8 flex-1 rounded-full text-xs"
              onClick={() => {
                onChange(draftStart, draftEnd)
                setOpen(false)
              }}
            >
              Apply
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
