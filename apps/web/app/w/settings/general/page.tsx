"use client"

import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { useIsAdmin, useIsOwner } from "@/lib/auth/use-role"
import { cn } from "@/lib/utils"
import { WorkspaceDangerZone } from "./workspace-danger-zone"
import { appendTranscriptToComposer } from "@/app/w/(chat)/_lib/audio-transcriptions"
import { useComposerAudioRecording } from "@/hooks/use-composer-audio-recording"
import { useOrgAudioTranscription } from "@/hooks/use-org-audio-transcription"
import { MicrophonePermissionModal } from "@/app/w/(chat)/_components/microphone-permission-modal"
import { RecordingWaveform } from "@/app/w/(chat)/_components/recording-waveform"

const MAX_LOGO_BYTES = 5 * 1024 * 1024 // 5 MB
const PROMPT_COMPANY_MAX = 4000

interface ProfileForm {
  name: string
  website: string
  promptCompany: string
  logoUrl: string
}

export default function GeneralSettingsPage() {
  const queryClient = useQueryClient()
  // Editing the workspace profile mutates org config (PATCH /v1/orgs/current,
  // POST /v1/uploads/sign for the logo), which is admin-only on the backend.
  // Non-admins can view the current settings but not change them; the
  // backend enforces this too.
  const isAdmin = useIsAdmin()
  // Transfer-ownership and delete-workspace are owner-only, org-wide actions —
  // rendered in a separate danger-zone section below, independent of the
  // admin-gated profile form above.
  const isOwner = useIsOwner()
  const orgQuery = $api.useQuery("get", "/v1/orgs/current")
  const updateOrg = $api.useMutation("patch", "/v1/orgs/current")
  const signUpload = $api.useMutation("post", "/v1/uploads/sign")

  const fileInputRef = useRef<HTMLInputElement>(null)
  const [uploadingLogo, setUploadingLogo] = useState(false)

  const saved = useMemo<ProfileForm>(
    () => ({
      name: orgQuery.data?.name ?? "",
      website: orgQuery.data?.website ?? "",
      promptCompany: orgQuery.data?.prompt_company ?? "",
      logoUrl: orgQuery.data?.logo_url ?? "",
    }),
    [orgQuery.data]
  )

  const [form, setForm] = useState<ProfileForm>(saved)

  // Sync local form when the org loads or is refetched.
  useEffect(() => {
    setForm(saved)
  }, [saved])

  const orgId = orgQuery.data?.id
  const nameError = form.name.trim().length === 0
  const dirty =
    form.name !== saved.name ||
    form.website !== saved.website ||
    form.promptCompany !== saved.promptCompany ||
    form.logoUrl !== saved.logoUrl
  const canSave =
    isAdmin &&
    !orgQuery.isLoading &&
    !updateOrg.isPending &&
    !nameError &&
    dirty

  function update<K extends keyof ProfileForm>(key: K, value: ProfileForm[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  // Voice-to-text for company context — same recorder UX as the chat
  // composer, transcribed through the org-scoped endpoint.
  const handleTranscript = useCallback((text: string) => {
    setForm((prev) => ({
      ...prev,
      promptCompany: appendTranscriptToComposer(prev.promptCompany, text).slice(
        0,
        PROMPT_COMPANY_MAX
      ),
    }))
  }, [])

  const {
    audioData,
    formattedRecordingTime,
    isProcessingStartRecording,
    isRecordingInProgress,
    isTranscribingRecording,
    micPromptOpen,
    recordingActive,
    setMicPromptOpen,
    startRecordingFromPrompt,
    toggleRecording,
  } = useComposerAudioRecording({
    isStreaming: false,
    onTranscript: handleTranscript,
    transcription: useOrgAudioTranscription(),
  })

  async function handleLogoSelect(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = "" // allow re-selecting the same file
    if (!file || !isAdmin) return

    if (!file.type.startsWith("image/")) {
      toast.danger("Choose an image file")
      return
    }
    if (file.size > MAX_LOGO_BYTES) {
      toast.danger("Logo must be 5 MB or smaller")
      return
    }

    setUploadingLogo(true)
    try {
      const signed = await signUpload.mutateAsync({
        body: {
          asset_type: "org_logo",
          content_type: file.type,
          filename: file.name,
          size_bytes: file.size,
          ...(orgId ? { org_id: orgId } : {}),
        },
      })
      if (!signed.upload_url || !signed.asset_url) {
        throw new Error("Upload could not be signed")
      }

      // eslint-disable-next-line no-restricted-globals -- signed-storage upload: PUT straight to the pre-signed object-store URL, not the Hivy API.
      const putRes = await fetch(signed.upload_url, {
        method: signed.upload_method || "PUT",
        headers: {
          "Content-Type": file.type,
          ...(signed.required_headers ?? {}),
        },
        body: file,
      })
      if (!putRes.ok) {
        throw new Error("Upload failed")
      }

      update("logoUrl", signed.asset_url)
      toast.success("Logo uploaded — save to apply")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not upload logo"))
    } finally {
      setUploadingLogo(false)
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSave) return
    updateOrg.mutate(
      {
        body: {
          name: form.name.trim(),
          website: form.website.trim(),
          prompt_company: form.promptCompany.trim(),
          logo_url: form.logoUrl,
        },
      },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({
            queryKey: queryKeys.orgCurrent(),
          })
          toast.success("Workspace updated")
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update workspace")
          ),
      }
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <form onSubmit={handleSubmit} className="flex flex-col gap-8">
        <div>
          <h1 className="text-lg font-semibold text-foreground">General</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Manage your workspace profile and the context your agents share.
          </p>
          {!isAdmin ? (
            <p className="text-muted-foreground mt-1 text-sm">
              Only workspace admins can edit workspace settings.
            </p>
          ) : null}
        </div>

        {/* Logo */}
        <section className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <h2 className="text-sm font-semibold text-foreground">Logo</h2>
            <p className="text-muted-foreground text-sm leading-5">
              A square image, at least 256×256px. Click to upload — PNG or SVG
              works best.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <button
              type="button"
              aria-label="Upload workspace logo"
              disabled={uploadingLogo || !isAdmin}
              onClick={() => fileInputRef.current?.click()}
              className="group relative flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-border bg-default transition-colors hover:border-accent disabled:cursor-not-allowed disabled:hover:border-border"
            >
              {form.logoUrl ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={form.logoUrl}
                  alt="Workspace logo"
                  className="h-full w-full object-cover"
                />
              ) : (
                <AppIcon
                  icon="image"
                  className="text-muted-foreground h-5 w-5"
                />
              )}

              {/* Hover edit affordance */}
              {isAdmin ? (
                <span className="absolute inset-0 flex items-center justify-center bg-black/45 opacity-0 transition-opacity group-hover:opacity-100">
                  <AppIcon icon="pencil" className="h-4 w-4 text-white" />
                </span>
              ) : null}

              {/* Uploading state */}
              {uploadingLogo ? (
                <span className="absolute inset-0 flex items-center justify-center bg-surface/70">
                  <Spinner color="current" size="sm" />
                </span>
              ) : null}
            </button>

            {isAdmin && form.logoUrl ? (
              <button
                type="button"
                onClick={() => update("logoUrl", "")}
                className="text-muted-foreground text-sm transition-colors hover:text-foreground"
              >
                Remove
              </button>
            ) : null}

            {isAdmin ? (
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={handleLogoSelect}
              />
            ) : null}
          </div>
        </section>

        {/* Name */}
        <section className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <h2 className="text-sm font-semibold text-foreground">Name</h2>
            <p className="text-muted-foreground text-sm leading-5">
              The name of your workspace.
            </p>
          </div>
          <Input
            aria-label="Workspace name"
            value={form.name}
            onChange={(event) => update("name", event.target.value)}
            placeholder="Acme Inc."
            className={cn("w-full", nameError && "border-danger/70")}
            aria-invalid={nameError || undefined}
            disabled={orgQuery.isLoading || !isAdmin}
            readOnly={!isAdmin}
          />
          {nameError ? (
            <p className="text-sm text-danger">Name can&apos;t be empty.</p>
          ) : null}
        </section>

        {/* Website */}
        <section className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <h2 className="text-sm font-semibold text-foreground">Website</h2>
            <p className="text-muted-foreground text-sm leading-5">
              Indexed into the knowledge base so agents can reference your site.
            </p>
          </div>
          <Input
            aria-label="Website"
            value={form.website}
            onChange={(event) => update("website", event.target.value)}
            placeholder="https://acme.com"
            className="w-full"
            disabled={orgQuery.isLoading || !isAdmin}
            readOnly={!isAdmin}
          />
        </section>

        {/* Company context */}
        <section className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <h2 className="text-sm font-semibold text-foreground">
              Company context
            </h2>
            <p className="text-muted-foreground text-sm leading-5">
              Shared background about your company — what you do, your product,
              your voice. Every agent uses this to understand your business.
            </p>
          </div>
          <div className="flex flex-col gap-2 rounded-3xl border border-border bg-surface px-3 pt-3 pb-2 shadow-sm transition-colors">
            <textarea
              aria-label="Company context"
              value={form.promptCompany}
              onChange={(event) =>
                update(
                  "promptCompany",
                  event.target.value.slice(0, PROMPT_COMPANY_MAX)
                )
              }
              rows={6}
              placeholder="Acme builds developer tools for fast-moving teams. Our tone is direct and friendly…"
              disabled={orgQuery.isLoading || !isAdmin}
              readOnly={!isAdmin}
              className="max-h-64 min-h-32 w-full resize-y overflow-y-auto bg-transparent px-2 text-[15px] leading-6 outline-none placeholder:text-muted disabled:cursor-not-allowed disabled:opacity-60"
            />

            <div className="flex items-center gap-1">
              {recordingActive ? (
                <div className="flex min-w-0 flex-1 items-center gap-3 pl-2">
                  <div className="h-9 min-w-0 flex-1 overflow-hidden">
                    <RecordingWaveform
                      active={isRecordingInProgress}
                      audioData={audioData}
                    />
                  </div>
                  <span className="shrink-0 text-sm font-medium text-muted tabular-nums">
                    {formattedRecordingTime || "00:00"}
                  </span>
                </div>
              ) : (
                <>
                  <span className="px-2 text-xs text-muted tabular-nums">
                    {form.promptCompany.length} / {PROMPT_COMPANY_MAX}
                  </span>
                  <div className="flex-1" />
                </>
              )}

              {!isAdmin ? null : isTranscribingRecording ? (
                <Button
                  variant="ghost"
                  size="sm"
                  isIconOnly
                  aria-label="Transcribing audio"
                  isDisabled
                >
                  <Spinner color="current" size="sm" className="text-muted" />
                </Button>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  isIconOnly
                  aria-label={
                    isRecordingInProgress ? "Stop recording" : "Record audio"
                  }
                  isDisabled={orgQuery.isLoading || isProcessingStartRecording}
                  onPress={() => void toggleRecording()}
                >
                  <AppIcon
                    icon={isRecordingInProgress ? "square" : "mic"}
                    className={`h-4 w-4 ${isRecordingInProgress ? "text-danger" : "text-muted"}`}
                  />
                </Button>
              )}
            </div>
          </div>
        </section>

        {/* Actions */}
        {isAdmin ? (
          <div className="flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="tertiary"
              size="sm"
              isDisabled={updateOrg.isPending || !dirty}
              onPress={() => setForm(saved)}
            >
              Reset
            </Button>
            <Button
              type="submit"
              variant="primary"
              size="sm"
              isDisabled={!canSave}
            >
              {updateOrg.isPending ? (
                <Spinner color="current" size="sm" />
              ) : null}
              Save changes
            </Button>
          </div>
        ) : null}

        <MicrophonePermissionModal
          open={micPromptOpen}
          onOpenChange={setMicPromptOpen}
          onConfirm={startRecordingFromPrompt}
        />
      </form>

      {isOwner ? <WorkspaceDangerZone /> : null}
    </div>
  )
}
