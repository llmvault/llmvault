"use client"

import { useEffect, useMemo, useState } from "react"
import { Input, ListBox, Select, TimeField } from "@heroui/react"
import { Time } from "@internationalized/date"
import { AppIcon } from "@/components/icon"
import type { ScheduleItem } from "@/app/w/(chat)/automations/_data"

type Frequency = "daily" | "weekly" | "hourly" | "interval" | "cron"

const FREQUENCY_LABELS: Record<Frequency, string> = {
  daily: "Every day",
  weekly: "Every week",
  hourly: "Every hour",
  interval: "Custom interval",
  cron: "Custom cron",
}

const WEEKDAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]

const UNIT_SECONDS: Record<string, number> = {
  minutes: 60,
  hours: 3600,
  days: 86400,
}

type CadenceBody = { cron_expression: string } | { interval_seconds: number }
export type Cadence =
  | { body: CadenceBody; preview: string; local: string }
  | { error: string }

type CadenceState = {
  frequency: Frequency
  time: Time
  weekdays: number[]
  minuteOfHour: string
  intervalValue: string
  intervalUnit: string
  cronExpr: string
}

function defaultCadenceState(): CadenceState {
  return {
    frequency: "weekly",
    time: new Time(9, 0),
    weekdays: [1],
    minuteOfHour: "0",
    intervalValue: "30",
    intervalUnit: "minutes",
    cronExpr: "0 9 * * 1",
  }
}

function pad(n: number): string {
  return String(n).padStart(2, "0")
}

function format12h(h: number, m: number): string {
  const period = h >= 12 ? "PM" : "AM"
  const hour = h % 12 === 0 ? 12 : h % 12
  return `${hour}:${pad(m)} ${period}`
}

// localToUtc maps a local wall-clock time (and optional local weekday) to the
// UTC fields a cron expression needs. Converting can shift the weekday when the
// time crosses midnight in UTC, so we read the weekday back from UTC too.
function localToUtc(h: number, m: number, weekday?: number) {
  const d = new Date()
  if (weekday !== undefined) {
    d.setDate(d.getDate() + ((weekday - d.getDay() + 7) % 7))
  }
  d.setHours(h, m, 0, 0)
  return { h: d.getUTCHours(), m: d.getUTCMinutes(), weekday: d.getUTCDay() }
}

// utcToLocal is the inverse of localToUtc — used to seed the editor from a
// stored (UTC) cron expression back into the user's local time.
function utcToLocal(h: number, m: number, weekday?: number) {
  const d = new Date()
  if (weekday !== undefined) {
    d.setUTCDate(d.getUTCDate() + ((weekday - d.getUTCDay() + 7) % 7))
  }
  d.setUTCHours(h, m, 0, 0)
  return { h: d.getHours(), m: d.getMinutes(), weekday: d.getDay() }
}

function buildCadence(args: {
  frequency: Frequency
  timeHour: number
  timeMinute: number
  weekdays: number[]
  minuteOfHour: number
  intervalSeconds: number
  intervalValue: string
  intervalUnit: string
  cronExpr: string
}): Cadence {
  const {
    frequency,
    timeHour,
    timeMinute,
    weekdays,
    minuteOfHour,
    intervalSeconds,
    intervalValue,
    intervalUnit,
    cronExpr,
  } = args

  if (frequency === "cron") {
    const expr = cronExpr.trim().replace(/\s+/g, " ")
    if (!expr) return { error: "Enter a cron expression" }
    if (expr.split(" ").length !== 5) {
      return { error: "Cron must have 5 fields (min hour day month weekday)" }
    }
    return {
      body: { cron_expression: expr },
      preview: `Runs on cron "${expr}" (UTC).`,
      local: "",
    }
  }

  if (frequency === "interval") {
    if (!(intervalSeconds > 0)) return { error: "Enter an interval" }
    const label = `${intervalValue} ${intervalUnit}`
    return {
      body: { interval_seconds: intervalSeconds },
      preview: `Runs every ${label} — a rolling interval that starts when you create it (UTC).`,
      local: `every ${label}`,
    }
  }

  if (frequency === "hourly") {
    const minute = Math.min(59, Math.max(0, minuteOfHour))
    const u = localToUtc(0, minute)
    return {
      body: { cron_expression: `${u.m} * * * *` },
      preview: `Runs every hour at :${pad(u.m)} past the hour (UTC).`,
      local: `every hour at :${pad(minute)}`,
    }
  }

  const h = timeHour
  const m = timeMinute

  if (frequency === "daily") {
    const u = localToUtc(h, m)
    return {
      body: { cron_expression: `${u.m} ${u.h} * * *` },
      preview: `Runs every day at ${pad(u.h)}:${pad(u.m)} UTC.`,
      local: `every day at ${format12h(h, m)}`,
    }
  }

  // weekly
  if (weekdays.length === 0) return { error: "Pick at least one day" }
  let uh = 0
  let um = 0
  const utcDays = new Set<number>()
  for (const wd of weekdays) {
    const u = localToUtc(h, m, wd)
    uh = u.h
    um = u.m
    utcDays.add(u.weekday)
  }
  const days = Array.from(utcDays).sort((a, b) => a - b)
  const localDays = [...weekdays].sort((a, b) => a - b)
  return {
    body: { cron_expression: `${um} ${uh} * * ${days.join(",")}` },
    preview: `Runs at ${pad(uh)}:${pad(um)} UTC on ${days
      .map((d) => WEEKDAY_LABELS[d])
      .join(", ")}.`,
    local: `every ${localDays.map((d) => WEEKDAY_LABELS[d]).join(", ")} at ${format12h(h, m)}`,
  }
}

// scheduleToCadenceState reverses a stored schedule (UTC cron / interval) back
// into the friendly editor state, falling back to raw cron when it doesn't fit
// one of the friendly shapes.
export function scheduleToCadenceState(schedule: ScheduleItem): CadenceState {
  const base = defaultCadenceState()

  if (schedule.interval_seconds && schedule.interval_seconds > 0) {
    const s = schedule.interval_seconds
    let unit = "minutes"
    let value = Math.round(s / 60)
    if (s % 86400 === 0) {
      unit = "days"
      value = s / 86400
    } else if (s % 3600 === 0) {
      unit = "hours"
      value = s / 3600
    }
    return {
      ...base,
      frequency: "interval",
      intervalValue: String(value),
      intervalUnit: unit,
    }
  }

  const expr = (schedule.cron_expression || "").trim().replace(/\s+/g, " ")
  if (!expr) return base
  const parts = expr.split(" ")
  if (parts.length === 5) {
    const [minS, hourS, dom, mon, dowS] = parts
    const numMin = /^\d+$/.test(minS)
    const numHour = /^\d+$/.test(hourS)
    const min = Number(minS)
    const hour = Number(hourS)

    if (numMin && numHour && dom === "*" && mon === "*") {
      if (dowS === "*") {
        const l = utcToLocal(hour, min)
        return { ...base, frequency: "daily", time: new Time(l.h, l.m) }
      }
      if (/^[0-6](,[0-6])*$/.test(dowS)) {
        const local = new Set<number>()
        let lh = 0
        let lm = 0
        for (const d of dowS.split(",").map(Number)) {
          const l = utcToLocal(hour, min, d)
          lh = l.h
          lm = l.m
          local.add(l.weekday)
        }
        return {
          ...base,
          frequency: "weekly",
          time: new Time(lh, lm),
          weekdays: Array.from(local),
        }
      }
    }

    if (numMin && hourS === "*" && dom === "*" && mon === "*" && dowS === "*") {
      const l = utcToLocal(0, min)
      return { ...base, frequency: "hourly", minuteOfHour: String(l.m) }
    }
  }

  return { ...base, frequency: "cron", cronExpr: expr }
}

export function ScheduleCadenceFields({
  initial,
  onChange,
}: {
  initial?: CadenceState
  onChange: (cadence: Cadence) => void
}) {
  const [state, setState] = useState<CadenceState>(
    () => initial ?? defaultCadenceState()
  )
  const [mounted, setMounted] = useState(false)
  useEffect(() => setMounted(true), [])
  const timezone = mounted
    ? Intl.DateTimeFormat().resolvedOptions().timeZone
    : ""

  const intervalSeconds =
    Math.round(Number(state.intervalValue) || 0) *
    (UNIT_SECONDS[state.intervalUnit] ?? 60)
  const cadence = useMemo(
    () =>
      buildCadence({
        frequency: state.frequency,
        timeHour: state.time.hour,
        timeMinute: state.time.minute,
        weekdays: state.weekdays,
        minuteOfHour: Number(state.minuteOfHour) || 0,
        intervalSeconds,
        intervalValue: state.intervalValue,
        intervalUnit: state.intervalUnit,
        cronExpr: state.cronExpr,
      }),
    [state, intervalSeconds]
  )
  useEffect(() => onChange(cadence), [cadence, onChange])

  function update(partial: Partial<CadenceState>) {
    setState((prev) => ({ ...prev, ...partial }))
  }
  function toggleWeekday(day: number) {
    setState((prev) => ({
      ...prev,
      weekdays: prev.weekdays.includes(day)
        ? prev.weekdays.filter((d) => d !== day)
        : [...prev.weekdays, day],
    }))
  }

  const { frequency } = state

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <FrequencySelect
          value={frequency}
          onChange={(value) => update({ frequency: value })}
        />

        {frequency === "weekly" ? (
          <div className="flex flex-wrap gap-1.5">
            {WEEKDAY_LABELS.map((label, idx) => {
              const on = state.weekdays.includes(idx)
              return (
                <button
                  type="button"
                  key={label}
                  onClick={() => toggleWeekday(idx)}
                  className={`h-9 w-10 rounded-md border text-xs font-medium transition-colors ${
                    on
                      ? "border-transparent bg-foreground text-background"
                      : "text-muted-foreground hover:bg-default border-border"
                  }`}
                >
                  {label}
                </button>
              )
            })}
          </div>
        ) : null}

        {frequency === "hourly" ? (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-sm">at minute</span>
            <Input
              type="number"
              min={0}
              max={59}
              value={state.minuteOfHour}
              onChange={(event) => update({ minuteOfHour: event.target.value })}
              className="h-9 w-20 rounded-md"
            />
            <span className="text-muted-foreground text-sm">past the hour</span>
          </div>
        ) : null}

        {frequency === "interval" ? (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-sm">every</span>
            <Input
              type="number"
              min={1}
              value={state.intervalValue}
              onChange={(event) => update({ intervalValue: event.target.value })}
              className="h-9 w-20 rounded-md"
            />
            <UnitSelect
              value={state.intervalUnit}
              onChange={(value) => update({ intervalUnit: value })}
            />
          </div>
        ) : null}

        {frequency === "cron" ? (
          <Input
            value={state.cronExpr}
            onChange={(event) => update({ cronExpr: event.target.value })}
            placeholder="0 9 * * 1"
            aria-label="Cron expression"
            className="h-9 min-w-[12rem] flex-1 rounded-md font-mono text-sm"
          />
        ) : null}
      </div>

      {frequency === "daily" || frequency === "weekly" ? (
        <TimeField
          aria-label="Time of day"
          value={state.time}
          onChange={(next) =>
            next && update({ time: new Time(next.hour, next.minute) })
          }
          fullWidth
        >
          <TimeField.Group fullWidth>
            <TimeField.Input>
              {(segment) => <TimeField.Segment segment={segment} />}
            </TimeField.Input>
          </TimeField.Group>
        </TimeField>
      ) : null}

      {mounted && "body" in cadence ? (
        <div className="flex gap-2.5 rounded-xl border border-sky-500/25 bg-sky-500/[0.06] px-3 py-2.5">
          <AppIcon icon="clock" className="mt-0.5 h-4 w-4 shrink-0 text-sky-500" />
          <div className="flex flex-col gap-0.5 text-sm leading-5">
            <span className="font-medium text-foreground">
              {cadence.preview}
            </span>
            <span className="text-muted-foreground">
              {cadence.local
                ? `You picked ${cadence.local}${timezone ? ` (${timezone})` : ""}. All schedules run in UTC.`
                : "Cron expressions are evaluated in UTC."}
            </span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function FrequencySelect({
  value,
  onChange,
}: {
  value: Frequency
  onChange: (value: Frequency) => void
}) {
  return (
    <Select
      aria-label="Frequency"
      selectedKey={value}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key) as Frequency)
      }}
      className="w-full sm:w-52"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-52 p-1.5">
        <ListBox>
          {(Object.keys(FREQUENCY_LABELS) as Frequency[]).map((key) => (
            <ListBox.Item key={key} id={key} textValue={FREQUENCY_LABELS[key]}>
              {FREQUENCY_LABELS[key]}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function UnitSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Select
      aria-label="Interval unit"
      selectedKey={value}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full sm:w-32"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-32 p-1.5">
        <ListBox>
          <ListBox.Item id="minutes" textValue="Minutes">
            Minutes
          </ListBox.Item>
          <ListBox.Item id="hours" textValue="Hours">
            Hours
          </ListBox.Item>
          <ListBox.Item id="days" textValue="Days">
            Days
          </ListBox.Item>
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
