import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import { DriveBrowserPreview, SearchDownloadPreview } from "./drive-previews"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function DriveLandingPage() {
  return (
    <main className="marketing-link-scope light min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "One file store for every Hivy agent.",
          "Keep the work beyond the sandbox.",
        ]}
        description="Upload files for an agent, let it read them in a sandbox, and keep the files it produces. Drive makes both inputs and outputs searchable across that agent’s sessions."
        primaryAction={{
          label: "Create free workspace",
          href: "/auth/signup",
        }}
        secondaryAction={{ label: "See Drive in action", href: "#file-flow" }}
        placeholderLabel="Agent Drive with searchable files"
      />

      <section
        id="file-flow"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid items-end gap-8 lg:grid-cols-[0.82fr_1.18fr]">
          <div>
            <SectionEyebrow>Inputs and outputs together</SectionEyebrow>
            <h2 className="mt-5 max-w-[650px] text-[clamp(2rem,4vw,4rem)] leading-[0.98] font-medium tracking-[-0.055em]">
              Keep every file an agent needs in one place.
            </h2>
          </div>
          <p className="max-w-[610px] text-[1.05rem] leading-7 text-muted lg:justify-self-end">
            Customer uploads, call recordings, reports, and agent-made files sit
            in the same library. Folders, file type, owner, size, and date stay
            attached after the sandbox closes.
          </p>
        </div>
        <div className="mt-14 bg-surface-secondary p-3 md:p-8 lg:p-12">
          <DriveBrowserPreview />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "upload",
              title: "Uploads ready for the agent",
              description:
                "Add the PDFs, images, audio, spreadsheets, and documents the job starts from.",
            },
            {
              icon: "file-plus",
              title: "Agent outputs saved beside them",
              description:
                "Reports, exports, notes, and other deliverables return to Drive instead of disappearing with the sandbox.",
            },
            {
              icon: "history",
              title: "File details stay attached",
              description:
                "Keep the folder, owner, type, size, and modified time that explain where each asset belongs.",
            },
          ]}
        />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="mx-auto max-w-[800px] text-center">
          <SectionEyebrow>Files in the agent’s work</SectionEyebrow>
          <h2 className="mt-5 text-[clamp(2.35rem,4.5vw,4.5rem)] leading-[0.95] font-medium tracking-[-0.055em]">
            Agents find the exact file, use it, and keep the result.
          </h2>
          <p className="mx-auto mt-6 max-w-[650px] text-base leading-7 text-muted">
            An agent can search by name, folder, type, or date, download the
            matching asset into its current sandbox, and upload useful output
            before the session ends.
          </p>
        </div>
        <div className="mt-14 bg-surface-secondary p-3 md:p-8 lg:p-12">
          <SearchDownloadPreview />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "search",
              title: "Find the exact asset",
              description:
                "Search filenames and metadata first, then use the returned asset ID instead of guessing from a similar name.",
            },
            {
              icon: "download",
              title: "Bring it into the sandbox",
              description:
                "Download the selected file into the live session so the agent can inspect, parse, or transform it.",
            },
            {
              icon: "save",
              title: "Write useful output back",
              description:
                "Save the finished report, export, archive, or document to Drive so the next session can use it.",
            },
          ]}
        />
      </section>

      <section className="mx-auto flex min-h-[520px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-surface-secondary text-foreground">
          <AppIcon icon="folder-open" size={24} />
        </span>
        <h2 className="mt-8 max-w-[760px] text-[clamp(2.3rem,4.5vw,4.25rem)] leading-[0.95] font-medium tracking-[-0.055em]">
          Give your first agent a file it can keep using.
        </h2>
        <p className="mt-6 max-w-[560px] text-base leading-7 text-muted">
          Create your free Hivy workspace. Upload a file to an agent, then let
          Drive keep the outputs worth using again.
        </p>
        <div className="mt-8 flex items-center gap-2">
          <Link href="/auth/signup">
            <Button size="sm">Create free workspace</Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
