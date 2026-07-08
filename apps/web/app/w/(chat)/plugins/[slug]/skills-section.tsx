import {
  PluginLogo,
  pluginLogoFrameClass,
  pluginLogoFrameStyle,
} from "@/components/plugin-logo"
import { type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"

type PluginSkill = NonNullable<ApiPlugin["skills"]>[number]

export function SkillsSection({
  plugin,
  skills,
}: {
  plugin: ApiPlugin
  skills: PluginSkill[]
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold text-foreground">Skills</h2>
      <div className="bg-card flex flex-col divide-y divide-border rounded-xl border border-border">
        {skills.map((skill, index) => (
          <div
            key={skill.name || index}
            className="flex items-start gap-3 px-3 py-2.5"
          >
            <PluginListLogo plugin={plugin} />
            <div className="min-w-0">
              <p className="text-sm leading-5 font-medium text-foreground">
                {skill.name || "Skill"}
              </p>
              <p className="text-muted-foreground text-sm leading-5">
                {skill.human_description ||
                  skill.description ||
                  "No description available."}
              </p>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

export function PluginListLogo({ plugin }: { plugin: ApiPlugin }) {
  return (
    <div
      className={pluginLogoFrameClass(
        plugin,
        "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
      )}
      style={pluginLogoFrameStyle(plugin)}
    >
      <PluginLogo plugin={plugin} size={24} iconSize={16} forceIconWhite />
    </div>
  )
}
