"use client"

import type { FormEvent } from "react"
import { Button, Input, Label } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function SecretGate({
  value,
  onChange,
  onSubmit,
}: {
  value: string
  onChange: (value: string) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
}) {
  return (
    <div className="flex min-h-full items-center justify-center px-6 py-10">
      <form
        onSubmit={onSubmit}
        className="bg-surface flex w-full max-w-sm flex-col gap-4 rounded-3xl border border-border p-6 shadow-sm"
      >
        <div className="flex flex-col gap-2">
          <div className="flex size-10 items-center justify-center rounded-2xl border border-border bg-background">
            <AppIcon icon="key-round" className="size-4" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Admin secret required</h2>
            <p className="mt-1 text-sm text-muted">
              Enter the backend admin secret to unlock setup actions.
            </p>
          </div>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="admin-secret">Admin secret</Label>
          <Input
            id="admin-secret"
            type="password"
            autoComplete="off"
            value={value}
            onChange={(event) => onChange(event.target.value)}
          />
        </div>
        <Button
          type="submit"
          variant="primary"
          isDisabled={value.trim().length === 0}
        >
          Continue
        </Button>
      </form>
    </div>
  )
}
