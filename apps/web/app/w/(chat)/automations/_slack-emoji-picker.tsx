"use client"

import { useMemo, useState } from "react"
import emojiData from "@emoji-mart/data/sets/15/native.json"
import type { Emoji as EmojiMartEmoji, EmojiMartData } from "@emoji-mart/data"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"

type SlackEmojiOption = {
  id: string
  name: string
  glyph: string
  searchText: string
}
type SlackEmojiGroup = {
  id: string
  options: SlackEmojiOption[]
}

const slackEmojiGroups = buildSlackEmojiGroups(emojiData as EmojiMartData)
const slackEmojiOptionsByID = new Map(
  slackEmojiGroups.flatMap((group) =>
    group.options.map((option) => [option.id, option] as const)
  )
)
const emojiCategoryLabels: Record<string, string> = {
  people: "People",
  nature: "Nature",
  foods: "Food",
  activity: "Activity",
  places: "Places",
  objects: "Objects",
  symbols: "Symbols",
  flags: "Flags",
}

export function SlackEmojiPicker({
  emojiName,
  emojiGlyph,
  onChange,
}: {
  emojiName: string
  emojiGlyph: string
  onChange: (name: string, glyph: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const searchText = normalizeEmojiSearch(query)
  const groups = useMemo(() => {
    if (!searchText) return slackEmojiGroups
    return slackEmojiGroups
      .map((group) => ({
        ...group,
        options: group.options.filter((option) =>
          option.searchText.includes(searchText)
        ),
      }))
      .filter((group) => group.options.length > 0)
  }, [searchText])

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger className="flex h-9 w-full items-center justify-between rounded-md border border-border px-3 text-left text-sm transition-colors hover:bg-muted/20">
        <span className="flex min-w-0 items-center gap-2">
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-default text-base">
            {emojiGlyph}
          </span>
          <span className="truncate font-medium">:{emojiName}:</span>
        </span>
        <AppIcon icon="chevron-down" className="h-4 w-4 text-muted" />
      </Popover.Trigger>
      <Popover.Content className="w-[22rem] overflow-hidden rounded-2xl border border-border p-0">
        <Popover.Dialog className="flex max-h-[26rem] w-[22rem] flex-col p-0">
          <div className="border-b border-border p-2">
            <label className="flex h-8 items-center gap-2 rounded-md border border-border px-2 text-sm">
              <AppIcon
                icon="search"
                className="h-4 w-4 shrink-0 text-muted"
              />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search emoji"
                className="placeholder:text-muted-foreground min-w-0 flex-1 bg-transparent text-foreground outline-none"
              />
            </label>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {groups.length === 0 ? (
              <div className="text-muted-foreground flex h-24 items-center justify-center text-sm">
                No emojis found
              </div>
            ) : (
              groups.map((group) => (
                <section key={group.id} className="mb-3 last:mb-0">
                  <h3 className="text-muted-foreground px-1 pb-1 text-xs font-medium">
                    {emojiCategoryLabels[group.id] ?? group.id}
                  </h3>
                  <div className="grid grid-cols-8 gap-1">
                    {group.options.map((emoji) => (
                      <button
                        key={emoji.id}
                        type="button"
                        aria-pressed={emoji.id === emojiName}
                        aria-label={`:${emoji.id}: ${emoji.name}`}
                        title={`:${emoji.id}:`}
                        className={
                          emoji.id === emojiName
                            ? "flex h-8 w-8 items-center justify-center rounded-md bg-accent/15 text-xl"
                            : "flex h-8 w-8 items-center justify-center rounded-md text-xl transition-colors hover:bg-muted/30"
                        }
                        onClick={() => {
                          onChange(emoji.id, emoji.glyph)
                          setOpen(false)
                        }}
                      >
                        <span className="leading-none">{emoji.glyph}</span>
                      </button>
                    ))}
                  </div>
                </section>
              ))
            )}
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

export function normalizeEmojiName(value: string): string {
  return value
    .trim()
    .replace(/^:+|:+$/g, "")
    .toLowerCase()
}

export function defaultEmojiGlyph(name: string): string {
  return (
    slackEmojiOptionsByID.get(normalizeEmojiName(name))?.glyph ?? `:${name}:`
  )
}

function normalizeEmojiSearch(value: string): string {
  return normalizeEmojiName(value).replace(/[\s-]+/g, "_")
}

function buildSlackEmojiGroups(data: EmojiMartData): SlackEmojiGroup[] {
  return data.categories
    .map((category) => ({
      id: category.id,
      options: category.emojis
        .map((emojiID) => buildSlackEmojiOption(data.emojis[emojiID]))
        .filter((emoji): emoji is SlackEmojiOption => Boolean(emoji)),
    }))
    .filter((group) => group.options.length > 0)
}

function buildSlackEmojiOption(
  emoji: EmojiMartEmoji | undefined
): SlackEmojiOption | null {
  const glyph = emoji?.skins[0]?.native
  if (!emoji || !glyph) return null
  const id = normalizeEmojiName(emoji.id)
  return {
    id,
    name: emoji.name,
    glyph,
    searchText: normalizeEmojiSearch(
      [id, emoji.name, ...(emoji.keywords ?? [])].join(" ")
    ),
  }
}
