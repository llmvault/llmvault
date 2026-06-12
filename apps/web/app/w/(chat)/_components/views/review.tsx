"use client"

import { PatchDiff } from "@pierre/diffs/react"
import { reviewPatches } from "../../_lib/static-data"

export function ReviewView() {
  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2 text-sm">
        <span className="font-medium">Edited 9 files</span>
        <span className="text-success">+66</span>
        <span className="text-danger">-77</span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-3">
        {reviewPatches.map((patch, index) => (
          <PatchDiff
            key={index}
            patch={patch}
            options={{ theme: "pierre-light" }}
            disableWorkerPool
          />
        ))}
      </div>
    </div>
  )
}
