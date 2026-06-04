import { HugeiconsIcon } from "@hugeicons/react"
import { DriveIcon } from "@hugeicons/core-free-icons"

export default function DrivePage() {
  return (
    <main className="flex min-h-full flex-1 flex-col px-6 py-8 md:px-10">
      <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col">
        <div className="flex items-center gap-3">
          <div className="flex size-10 items-center justify-center border border-border bg-surface-muted text-foreground">
            <HugeiconsIcon icon={DriveIcon} size={20} aria-hidden="true" />
          </div>
          <div>
            <h1 className="font-heading text-3xl font-normal text-foreground">
              Drive
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Files produced by your employee will appear here.
            </p>
          </div>
        </div>

        <div className="mt-10 flex flex-1 items-center justify-center border border-dashed border-border bg-surface-muted/40 px-6 py-16 text-center">
          <div className="max-w-sm">
            <p className="text-sm font-medium text-foreground">
              No files yet
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Ask your employee to create a report, screenshot, export, or
              other artifact. Saved files will be stored in this drive.
            </p>
          </div>
        </div>
      </div>
    </main>
  )
}
