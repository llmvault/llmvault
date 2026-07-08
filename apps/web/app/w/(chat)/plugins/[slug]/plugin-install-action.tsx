"use client"

import { Button, Spinner, Tooltip } from "@heroui/react"
import { pluginName, type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"

export function PluginInstallAction({
  plugin,
  busy,
  canInstall,
  canManage = true,
  onInstall,
  onUninstall,
}: {
  plugin: ApiPlugin
  busy: boolean
  canInstall: boolean
  // canManage gates install/remove to org admins. Non-admins still see the
  // plugin (and whether it's installed) but can't change installation; the
  // backend enforces this too.
  canManage?: boolean
  onInstall: () => void
  onUninstall: () => void
}) {
  const globalInstall = plugin.auto_install === true
  const lockedInstall = plugin.locked === true
  const cannotRemove =
    plugin.installed === true && (globalInstall || lockedInstall)
  const disabled =
    !canManage || busy || cannotRemove || (!plugin.installed && !canInstall)
  const label = plugin.installed ? "Remove" : "Add"
  const control = (
    <Button
      size="sm"
      variant="primary"
      isDisabled={disabled}
      aria-label={`${label} ${pluginName(plugin)} plugin`}
      onPress={plugin.installed ? onUninstall : onInstall}
    >
      {busy ? <Spinner color="current" size="sm" /> : null}
      {label}
    </Button>
  )

  const tooltip = !canManage
    ? "Only workspace admins can install or remove plugins."
    : cannotRemove
      ? globalInstall
        ? "This plugin is installed for all agents and cannot be removed."
        : "This plugin is required and cannot be removed."
      : null

  if (!tooltip) {
    return control
  }

  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">{control}</Tooltip.Trigger>
      <Tooltip.Content placement="left" offset={8} className="text-xs">
        {tooltip}
      </Tooltip.Content>
    </Tooltip>
  )
}
