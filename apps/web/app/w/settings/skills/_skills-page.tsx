"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import {
  Button,
  Input,
  Modal,
  TextArea,
  buttonVariants,
  toast,
} from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { AppIcon } from "@/components/icon"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import {
  EmptyProvisioningRow,
  ProvisioningSkeleton,
} from "../teams/[teamId]/_provisioning-row"

type Skill = components["schemas"]["skillResponse"]

export default function SkillsPageContent() {
  const queryClient = useQueryClient()
  const skillsQuery = $api.useQuery("get", "/v1/skills")
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const [editing, setEditing] = useState<Skill | null>(null)
  const [deleting, setDeleting] = useState<Skill | null>(null)
  const skills = skillsQuery.data?.skills ?? []
  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
  )
  const teamNamesByID = useMemo(
    () =>
      new Map(
        teams.flatMap((team) =>
          team.id ? ([[team.id, team.name || "Team"]] as const) : []
        )
      ),
    [teams]
  )
  const archive = $api.useMutation("delete", "/v1/skills/{id}", {
    onSuccess: () => {
      toast.success("Skill archived")
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.skills() })
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not archive skill")),
  })

  return (
    <div className="flex flex-col gap-8">
      <SkillsHeader />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {skillsQuery.isLoading || teamsQuery.isLoading ? (
          <ProvisioningSkeleton />
        ) : skills.length === 0 ? (
          <EmptyProvisioningRow text="No skills exist for this workspace yet." />
        ) : (
          <div className="flex flex-col divide-y divide-border">
            {skills.map((skill) => (
              <div key={skill.id} className="flex items-center gap-4 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{skill.name}</p>
                  <p className="truncate text-xs text-muted">
                    {skillScopeLabel(skill, teamNamesByID)}
                    {" · "}
                    {skill.human_description || skill.description || skill.slug}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  onPress={() => setEditing(skill)}
                >
                  Edit
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onPress={() => setDeleting(skill)}
                >
                  Archive
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
      <SkillDialog
        skill={editing}
        onClose={() => setEditing(null)}
        onSaved={() => {
          setEditing(null)
          void queryClient.invalidateQueries({ queryKey: queryKeys.skills() })
        }}
      />
      <ConfirmDialog
        open={deleting !== null}
        pending={archive.isPending}
        heading="Archive skill?"
        description="Agents will no longer receive this skill."
        confirmLabel="Archive"
        onOpenChange={(open) => !open && setDeleting(null)}
        onConfirm={() =>
          deleting?.id &&
          archive.mutate({ params: { path: { id: deleting.id } } })
        }
      />
    </div>
  )
}

export function SkillsHeader() {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <h1 className="text-lg font-semibold">Skills</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Reusable instructions available to agents. Team members manage their
          team skills; admins can also manage workspace-wide skills.
        </p>
      </div>
      <NextLink
        href="/w"
        className={buttonVariants({ size: "sm", variant: "primary" })}
      >
        <AppIcon icon="plus" className="h-4 w-4" />
        Add skill
      </NextLink>
    </div>
  )
}

function skillScopeLabel(skill: Skill, teamNamesByID: Map<string, string>) {
  if (!skill.team_id) return "Workspace-wide"
  return `Team: ${teamNamesByID.get(skill.team_id) ?? "Unknown team"}`
}

function SkillDialog({
  skill,
  onClose,
  onSaved,
}: {
  skill: Skill | null
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(skill?.name ?? "")
  const [slug, setSlug] = useState(skill?.slug ?? "")
  const [description, setDescription] = useState(
    skill?.human_description ?? skill?.description ?? ""
  )
  const [content, setContent] = useState(() => {
    const bundle = skill?.bundle as { content?: string } | undefined
    return bundle?.content ?? ""
  })
  const update = $api.useMutation("patch", "/v1/skills/{id}")
  if (!skill) return null
  const save = async () => {
    const body = {
      name,
      slug,
      human_description: description,
      description,
      team_id: skill.team_id,
      status: "published",
      bundle: {
        id: slug,
        title: name,
        description,
        content,
        files: {},
        references: [],
      },
    }
    try {
      if (!skill.id) return
      await update.mutateAsync({ params: { path: { id: skill.id } }, body })
      toast.success("Skill updated")
      onSaved()
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not save skill"))
    }
  }
  return (
    <Modal isOpen onOpenChange={(open) => !open && onClose()}>
      <Modal.Backdrop>
        <Modal.Container placement="center">
          <Modal.Dialog className="w-full max-w-xl p-6">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>Edit skill</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-4">
              <Input
                aria-label="Name"
                placeholder="Skill name"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
              <Input
                aria-label="Slug"
                placeholder="skill-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
              />
              <Input
                aria-label="Description"
                placeholder="When should agents use this skill?"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
              <TextArea
                aria-label="Instructions"
                placeholder="Write the skill instructions…"
                value={content}
                onChange={(e) => setContent(e.target.value)}
              />
            </Modal.Body>
            <Modal.Footer>
              <Button variant="ghost" onPress={onClose}>
                Cancel
              </Button>
              <Button
                variant="primary"
                isDisabled={!name.trim() || !slug.trim() || !content.trim()}
                isPending={update.isPending}
                onPress={save}
              >
                Save skill
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
