"use client"

import { useState, useCallback } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { HugeiconsIcon } from "@hugeicons/react"
import { Alert02Icon } from "@hugeicons/core-free-icons"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { ImagePicker } from "@/components/image-picker"
import { SettingsShell } from "@/components/settings-shell"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { extractErrorMessage } from "@/lib/api/error"

export default function GeneralSettingsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()

  const [name, setName] = useState(user?.name ?? "")
  const [email, setEmail] = useState(user?.email ?? "")
  const [avatarUrl, setAvatarUrl] = useState<string | undefined>(user?.avatar_url)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const updateMutation = $api.useMutation("patch", "/auth/me")
  const deleteMutation = $api.useMutation("delete", "/auth/me")

  const hasChanges =
    name !== (user?.name ?? "") ||
    email !== (user?.email ?? "") ||
    avatarUrl !== user?.avatar_url

  const handleSave = useCallback(() => {
    const body: Record<string, string> = {}
    if (name !== (user?.name ?? "")) body.name = name
    if (email !== (user?.email ?? "")) body.email = email
    if (avatarUrl !== (user?.avatar_url ?? undefined)) body.avatar_url = avatarUrl ?? ""

    const emailChanged = Boolean(body.email)

    updateMutation.mutate(
      { body: body as never },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
          if (emailChanged) {
            toast.success("Check your new email for a confirmation code")
          } else {
            toast.success("Profile updated")
          }
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Failed to update profile"))
        },
      }
    )
  }, [name, email, avatarUrl, user, updateMutation, queryClient])

  const handleDelete = useCallback(() => {
    deleteMutation.mutate(
      {},
      {
        onSuccess: () => {
          queryClient.clear()
          window.location.href = "/auth/signin"
        },
        onError: () => {
          toast.error("Failed to delete account")
        },
      }
    )
  }, [deleteMutation, queryClient])

  return (
    <SettingsShell title="General" description="Manage your personal profile and account settings.">
      <section>
        <div className="flex items-start gap-6">
          <div className="flex flex-col gap-4">
            <h2 className="text-sm font-medium text-foreground">Profile picture</h2>
            <p className="text-xs text-muted-foreground">
              A square image helps others recognize you. JPEG, PNG, or WebP, max 5 MB.
            </p>
          </div>
          <div className="flex shrink-0 items-center">
            <ImagePicker
              value={avatarUrl}
              onChange={setAvatarUrl}
              assetType="avatar"
              fallback={user?.name?.charAt(0) ?? "?"}
              ariaLabel="Upload profile picture"
            />
          </div>
        </div>
      </section>

      <Separator />

      <section>
        <h2 className="text-sm font-medium text-foreground">Personal information</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          Your name and email address are used across your account.
        </p>
        <div className="mt-5 flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="name" className="text-xs">
              Name
            </Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Your name"
              className="max-w-sm"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="email" className="text-xs">
              Email
            </Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              className="max-w-sm"
            />
          </div>
        </div>
        <div className="mt-6">
          <Button
            onClick={handleSave}
            loading={updateMutation.isPending}
            disabled={!hasChanges}
            size="sm"
          >
            Save changes
          </Button>
        </div>
      </section>

      <Separator />

      <section>
        <div className="flex items-start gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
          <HugeiconsIcon
            icon={Alert02Icon}
            className="mt-0.5 size-5 shrink-0 text-destructive"
          />
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-medium text-foreground">Delete account</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Permanently delete your account and all associated data. This action
              cannot be undone.
            </p>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setDeleteOpen(true)}
              className="mt-3"
            >
              Delete account
            </Button>
          </div>
        </div>
      </section>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete account"
        description="This will permanently delete your account and all associated data. This action cannot be undone."
        confirmLabel="Delete account"
        destructive
        loading={deleteMutation.isPending}
        onConfirm={handleDelete}
        confirmText={user?.email ?? ""}
        confirmTextLabel={`Type your email to confirm`}
      />
    </SettingsShell>
  )
}
