import { readFileSync } from "node:fs"
import { describe, expect, it } from "vitest"

const marketingTabSources = [
  "./access-control/_components/access-control-scenes.tsx",
  "./agents/_components/agent-catalog-explorer.tsx",
  "./automations/_components/automation-previews.tsx",
  "./drive/_components/drive-previews.tsx",
  "./knowledge/_components/knowledge-scenes.tsx",
  "./sheets/_components/sheets-previews.tsx",
  "./tag/_components/slack-use-case-previews.tsx",
] as const

describe("marketing page tabs", () => {
  it.each(marketingTabSources)(
    "renders a HeroUI selection indicator in every tab from %s",
    (sourcePath) => {
      const source = readFileSync(new URL(sourcePath, import.meta.url), "utf8")
      const tabBlocks = source.match(/<Tabs\.Tab\b[\s\S]*?<\/Tabs\.Tab>/g)

      expect(tabBlocks).not.toBeNull()
      for (const tabBlock of tabBlocks ?? []) {
        expect(tabBlock).toContain("<Tabs.Indicator />")
      }
    }
  )
})
