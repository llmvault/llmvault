"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons"
import { cn } from "@/lib/utils"

const DAYS = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"]

function toDateString(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${y}-${m}-${day}`
}

function buildMonthGrid(year: number, month: number): (number | null)[][] {
  const firstDay = new Date(year, month, 1).getDay()
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const weeks: (number | null)[][] = []
  let week: (number | null)[] = new Array(firstDay).fill(null)
  for (let d = 1; d <= daysInMonth; d++) {
    week.push(d)
    if (week.length === 7) {
      weeks.push(week)
      week = []
    }
  }
  if (week.length > 0) {
    while (week.length < 7) week.push(null)
    weeks.push(week)
  }
  return weeks
}

export function Calendar({
  startDate,
  endDate,
  onSelect,
}: {
  startDate: string
  endDate: string
  onSelect: (date: string) => void
}) {
  const today = toDateString(new Date())
  const initialBase = startDate || endDate || today
  const initialDate = new Date(initialBase + "T00:00:00")
  const [viewYear, setViewYear] = useState(initialDate.getFullYear())
  const [viewMonth, setViewMonth] = useState(initialDate.getMonth())

  const weeks = buildMonthGrid(viewYear, viewMonth)
  const monthLabel = new Date(viewYear, viewMonth).toLocaleDateString("en-US", {
    month: "long",
    year: "numeric",
  })

  function goPrev() {
    if (viewMonth === 0) {
      setViewYear(viewYear - 1)
      setViewMonth(11)
    } else {
      setViewMonth(viewMonth - 1)
    }
  }

  function goNext() {
    if (viewMonth === 11) {
      setViewYear(viewYear + 1)
      setViewMonth(0)
    } else {
      setViewMonth(viewMonth + 1)
    }
  }

  function inRange(d: string): boolean {
    if (!startDate || !endDate) return false
    return d >= startDate && d <= endDate
  }

  return (
    <div className="flex flex-col gap-2">
      {/* Month header */}
      <div className="flex items-center justify-between">
        <button
          onClick={goPrev}
          className="flex size-7 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} className="size-3.5" />
        </button>
        <span className="text-sm font-medium text-foreground">{monthLabel}</span>
        <button
          onClick={goNext}
          className="flex size-7 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <HugeiconsIcon icon={ArrowRight01Icon} className="size-3.5" />
        </button>
      </div>

      {/* Day headers */}
      <div className="grid grid-cols-7 gap-0.5">
        {DAYS.map((d) => (
          <div
            key={d}
            className="flex h-8 items-center justify-center text-xs font-medium text-muted-foreground"
          >
            {d}
          </div>
        ))}
      </div>

      {/* Day grid — fixed height prevents popover repositioning when weeks change */}
      <div className="grid grid-cols-7 gap-0.5" style={{ minHeight: 200 }}>
        {weeks.map((week, wi) =>
          week.map((day, di) => {
            if (day == null) return <div key={`${wi}-${di}`} />

            const dateStr = toDateString(new Date(viewYear, viewMonth, day))
            const isStart = dateStr === startDate
            const isEnd = dateStr === endDate
            const inRangeActive = inRange(dateStr)
            const isToday = dateStr === today

            const isInRange = inRangeActive && !isStart && !isEnd

            return (
              <button
                key={`${wi}-${di}`}
                onClick={() => onSelect(dateStr)}
                className={cn(
                  "relative flex h-8 items-center justify-center text-sm transition-colors",
                  isStart && "rounded-l-full bg-foreground text-background",
                  isEnd && "rounded-r-full bg-foreground text-background",
                  isInRange && "bg-primary/10 text-foreground",
                  !isStart && !isEnd && !inRangeActive && "text-foreground hover:bg-muted",
                  isToday && !isStart && !isEnd && "font-semibold",
                )}
              >
                {day}
              </button>
            )
          }),
        )}
      </div>
    </div>
  )
}
