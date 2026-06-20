import { Icon } from "@iconify/react"
import { useState } from "react"

export function ActionsBlock() {
  const [copied, setCopied] = useState(false)
  const [vote, setVote] = useState<"up" | "down" | null>(null)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(
        "Fixed and pushed to main. Root cause was the CI file-length cap."
      )
    } catch {
      // Clipboard can be unavailable (permissions, insecure context); the
      // checkmark still gives design-exploration feedback.
    }
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="-mt-2 flex items-center gap-1">
      <ActionIcon
        icon={copied ? "lucide:check" : "lucide:copy"}
        label="Copy"
        active={copied}
        onPress={copy}
      />
      <ActionIcon
        icon="lucide:thumbs-up"
        label="Good response"
        active={vote === "up"}
        onPress={() => setVote((value) => (value === "up" ? null : "up"))}
      />
      <ActionIcon
        icon="lucide:thumbs-down"
        label="Bad response"
        active={vote === "down"}
        onPress={() => setVote((value) => (value === "down" ? null : "down"))}
      />
      <ActionIcon icon="lucide:share" label="Share" onPress={() => {}} />
    </div>
  )
}

function ActionIcon({
  icon,
  label,
  active,
  onPress,
}: {
  icon: string
  label: string
  active?: boolean
  onPress: () => void
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onPress}
      className={`hover:bg-default rounded-lg p-1.5 transition-colors ${
        active ? "text-foreground" : "text-muted hover:text-foreground"
      }`}
    >
      <Icon icon={icon} className="h-4 w-4" />
    </button>
  )
}
