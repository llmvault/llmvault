"use client"

import { Input, Label } from "@heroui/react"
import { cn } from "@/lib/utils"

export function Field({
  id,
  label,
  value,
  onChange,
  type = "text",
  placeholder,
  required,
  multiline,
}: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  type?: string
  placeholder?: string
  required?: boolean
  multiline?: boolean
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      {multiline ? (
        <textarea
          id={id}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          required={required}
          className={cn(
            "bg-field-background min-h-28 resize-y rounded-2xl border border-border px-3 py-2 text-sm transition-colors outline-none",
            "placeholder:text-muted focus:border-foreground/30 focus:bg-background"
          )}
        />
      ) : (
        <Input
          id={id}
          type={type}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          required={required}
        />
      )}
    </div>
  )
}
