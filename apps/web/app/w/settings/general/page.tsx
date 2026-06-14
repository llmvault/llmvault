"use client"

import { useState } from "react"
import { Button } from "@heroui/react"
import {
  PlainSwitch,
  Segmented,
  SettingRow,
  SettingSelect,
  ToggleRow,
  WorkModeCard,
} from "../_components/controls"

export default function GeneralSettingsPage() {
  const [workMode, setWorkMode] = useState<"coding" | "everyday">("coding")
  const [terminalLocation, setTerminalLocation] = useState<"Bottom" | "Right">(
    "Bottom"
  )
  const [codeReview, setCodeReview] = useState<"Inline" | "Detached">("Inline")

  return (
    <div className="flex flex-col gap-10">
      <h1 className="text-2xl font-semibold">General</h1>

      <section className="flex flex-col gap-3">
        <div>
          <h2 className="text-sm font-medium">Work mode</h2>
          <p className="text-sm text-muted">
            Choose how much technical detail Hivy shows
          </p>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <WorkModeCard
            icon="lucide:square-code"
            title="For coding"
            description="More technical responses and control"
            selected={workMode === "coding"}
            onSelect={() => setWorkMode("coding")}
          />
          <WorkModeCard
            icon="lucide:briefcase"
            title="For everyday work"
            description="Same power, less technical detail"
            selected={workMode === "everyday"}
            onSelect={() => setWorkMode("everyday")}
          />
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">Permissions</h2>
        <div className="rounded-2xl border border-border bg-surface">
          <ToggleRow
            title="Default permissions"
            description="By default, Hivy can read and edit files in its workspace. It can ask for additional access when needed"
            defaultSelected
          />
          <ToggleRow
            title="Auto-review"
            description="Hivy can read and edit files in its workspace. Hivy automatically reviews requests for additional access. Auto-review can make mistakes."
            learnMore
            defaultSelected
          />
          <ToggleRow
            title="Full access"
            description="When Hivy runs with full access, it can edit any file on your computer and run commands with network, without your approval. This significantly increases the risk of data loss, leaks, or unexpected behavior."
            learnMore
            defaultSelected
            last
          />
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">General</h2>
        <div className="rounded-2xl border border-border bg-surface">
          <SettingRow
            title="Default open destination"
            description="Where files and folders open by default"
          >
            <SettingSelect
              icon="vscode-icons:file-type-vscode"
              options={["VS Code", "Cursor", "Zed", "Finder"]}
            />
          </SettingRow>
          <SettingRow title="Language" description="Language for the app UI">
            <SettingSelect
              options={["Auto Detect", "English", "Deutsch", "Français"]}
            />
          </SettingRow>
          <SettingRow
            title="Show in menu bar"
            description="Keep Hivy in the macOS menu bar when the main window is closed"
          >
            <PlainSwitch defaultSelected />
          </SettingRow>
          <SettingRow
            title="Bottom panel"
            description="Show the bottom panel control in the app header"
          >
            <PlainSwitch defaultSelected />
          </SettingRow>
          <SettingRow
            title="Default terminal location"
            description="Choose where the terminal shortcut and environment actions open terminal tabs"
          >
            <Segmented
              options={["Bottom", "Right"]}
              value={terminalLocation}
              onChange={(value) =>
                setTerminalLocation(value as "Bottom" | "Right")
              }
            />
          </SettingRow>
          <SettingRow
            title="Prevent sleep while running"
            description="Keep your computer awake while Hivy is running a chat"
          >
            <PlainSwitch />
          </SettingRow>
          <SettingRow
            title="Speed"
            description="Choose the inference tier used across chats, subagents, and compaction"
          >
            <SettingSelect options={["Standard", "Flex", "Priority"]} />
          </SettingRow>
          <SettingRow
            title="Code review"
            description="Start /review in the current chat when possible or launch a separate review chat"
          >
            <Segmented
              options={["Inline", "Detached"]}
              value={codeReview}
              onChange={(value) => setCodeReview(value as "Inline" | "Detached")}
            />
          </SettingRow>
          <SettingRow
            title="Import work from other AI apps"
            description="Bring over your setup, projects, and recent chats"
            last
          >
            <Button variant="tertiary" size="sm">
              Import
            </Button>
          </SettingRow>
        </div>
      </section>
    </div>
  )
}
