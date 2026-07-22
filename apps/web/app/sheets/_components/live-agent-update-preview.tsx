"use client"

import type { ReactNode } from "react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import {
  leadRows,
  reveal,
  SheetShell,
  SheetToolbar,
  Status,
} from "./sheets-preview-primitives"

function SessionMessage({
  actor,
  children,
  icon,
}: {
  actor: string
  children: ReactNode
  icon: string
}) {
  return (
    <div className="flex gap-3 py-3">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-sm border border-border bg-surface">
        <AppIcon icon={icon} size={15} />
      </span>
      <div className="min-w-0">
        <p className="text-xs font-medium">{actor}</p>
        <div className="mt-1 text-xs leading-5 text-muted">{children}</div>
      </div>
    </div>
  )
}

export function LiveAgentUpdatePreview() {
  const newRows = leadRows.slice(0, 3)

  return (
    <MotionConfig reducedMotion="user">
      <div
        data-testid="sheet-agent-update-grid"
        className="grid overflow-hidden rounded-sm border border-border bg-surface shadow-surface lg:grid-cols-[1fr_2fr]"
      >
        <div className="border-b border-border p-5 lg:border-r lg:border-b-0 lg:p-7">
          <div className="flex items-center justify-between border-b border-border pb-3">
            <span className="text-xs font-medium">Renewal agent</span>
            <span className="text-[0.68rem] text-muted">Account review</span>
          </div>
          <SessionMessage actor="You" icon="circle-user">
            Review today’s account notes. Add each renewal with an owner and a
            clear next step.
          </SessionMessage>
          <motion.div
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, amount: 0.4 }}
          >
            <motion.div variants={reveal} custom={0.45}>
              <SessionMessage actor="Renewal agent" icon="bot">
                I found three accounts with a named owner and next step. I added
                them to{" "}
                <span className="font-medium text-foreground">
                  Renewal review
                </span>{" "}
                without changing the other records.
              </SessionMessage>
            </motion.div>
            <motion.div
              variants={reveal}
              custom={0.85}
              className="mt-2 rounded-sm bg-success/15 px-3 py-2 text-[0.68rem] text-success"
            >
              <span className="inline-flex items-center gap-1.5">
                <AppIcon icon="check" size={12} /> Added 3 account records
              </span>
            </motion.div>
          </motion.div>
        </div>

        <div className="min-w-0 bg-surface-secondary p-4 md:p-6">
          <SheetShell>
            <div className="overflow-x-auto">
              <SheetToolbar compact />
            </div>
            <div className="overflow-x-auto">
              <div className="min-w-[560px]">
                <div className="grid grid-cols-[36px_1fr_0.8fr_0.8fr_1.2fr] border-b border-border bg-surface-secondary text-[0.64rem] font-medium text-muted">
                  <span className="px-2 py-2">#</span>
                  <span className="border-l border-border px-3 py-2">
                    Company
                  </span>
                  <span className="border-l border-border px-3 py-2">
                    Status
                  </span>
                  <span className="border-l border-border px-3 py-2">
                    Owner
                  </span>
                  <span className="border-l border-border px-3 py-2">
                    Next step
                  </span>
                </div>
                <motion.div
                  initial="hidden"
                  whileInView="show"
                  viewport={{ once: true, amount: 0.35 }}
                >
                  {newRows.map((row, index) => (
                    <motion.div
                      key={row.company}
                      variants={reveal}
                      custom={0.35 + index * 0.28}
                      className="grid grid-cols-[36px_1fr_0.8fr_0.8fr_1.2fr] border-b border-border bg-surface text-[0.7rem] last:border-b-0"
                    >
                      <span className="px-2 py-3 text-muted">{index + 1}</span>
                      <span className="border-l border-border px-3 py-3 font-medium">
                        {row.company}
                      </span>
                      <span className="border-l border-border px-3 py-2.5">
                        <Status value={row.status} />
                      </span>
                      <span className="border-l border-border px-3 py-3">
                        {row.owner}
                      </span>
                      <span className="border-l border-border px-3 py-3 text-muted">
                        {row.next}
                      </span>
                    </motion.div>
                  ))}
                </motion.div>
              </div>
            </div>
          </SheetShell>
        </div>
      </div>
    </MotionConfig>
  )
}
