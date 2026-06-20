export function PersonAvatar({
  person,
  size = "sm",
}: {
  person: { initials: string; color: string }
  size?: "xs" | "sm"
}) {
  const dims = size === "xs" ? "h-5 w-5 text-[10px]" : "h-6 w-6 text-xs"

  return (
    <span
      className={`flex ${dims} ring-surface shrink-0 items-center justify-center rounded-full font-semibold text-white ring-2`}
      style={{ backgroundColor: person.color }}
    >
      {person.initials}
    </span>
  )
}
