import {
  Hash,
  Github,
  Folder,
  FileText,
  Database,
  Users,
  type LucideIcon,
} from 'lucide-react'

// Map of icon names from actions.json to Lucide icons
const iconMap: Record<string, LucideIcon> = {
  // Slack
  hash: Hash,
  
  // GitHub
  repo: Github,
  
  // Google Drive
  folder: Folder,
  
  // Notion
  page: FileText,
  database: Database,
  
  // Linear
  team: Users,
}

interface Props {
  iconName?: string
  size?: number
  className?: string
}

export function ResourceIcon({ iconName, size = 20, className }: Props) {
  if (!iconName) return null
  
  const Icon = iconMap[iconName.toLowerCase()]
  
  if (!Icon) {
    // Fallback to a generic icon if the icon name is not recognized
    return (
      <div 
        className={`flex items-center justify-center rounded-lg bg-cw-surface border border-solid border-cw-border ${className || ''}`}
        style={{ width: size * 2, height: size * 2 }}
      >
        <span className="text-lg">📁</span>
      </div>
    )
  }
  
  return (
    <div 
      className={`flex items-center justify-center rounded-lg bg-cw-surface border border-solid border-cw-border ${className || ''}`}
      style={{ width: size * 2, height: size * 2 }}
    >
      <Icon size={size} className="text-cw-heading" />
    </div>
  )
}
