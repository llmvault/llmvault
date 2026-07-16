"use client"

import { useMemo, useState } from "react"
import {
  Button,
  Input,
  ListBox,
  Modal,
  Select,
  TextArea,
  toast,
} from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { AppIcon } from "@/components/icon"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import { useIsAdmin } from "@/lib/auth/use-role"

type Skill = components["schemas"]["skillResponse"]

export default function SkillsSettingsPage() {
  const queryClient = useQueryClient()
  const isAdmin = useIsAdmin()
  const skillsQuery = $api.useQuery("get", "/v1/skills")
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const [editing, setEditing] = useState<Skill | "new" | null>(null)
  const [deleting, setDeleting] = useState<Skill | null>(null)
  const skills = skillsQuery.data?.skills ?? []
  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
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
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold">Skills</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Reusable instructions available to agents. Team members manage their
            team skills; admins can also manage workspace-wide skills.
          </p>
        </div>
        <Button size="sm" variant="primary" onPress={() => setEditing("new")}>
          <AppIcon icon="plus" className="h-4 w-4" />
          Add skill
        </Button>
      </div>
      {skillsQuery.isLoading ? (
        <p className="text-sm text-muted">Loading skills…</p>
      ) : skills.length === 0 ? (
        <div className="bg-card rounded-xl p-10 text-center text-sm text-muted">
          No skills yet.
        </div>
      ) : (
        <div className="bg-card flex flex-col divide-y divide-border rounded-xl">
          {skills.map((skill) => (
            <div key={skill.id} className="flex items-center gap-4 px-4 py-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{skill.name}</p>
                <p className="truncate text-xs text-muted">
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
      <SkillDialog
        skill={editing}
        teams={teams}
        isAdmin={isAdmin}
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

function SkillDialog({
  skill,
  teams,
  isAdmin,
  onClose,
  onSaved,
}: {
  skill: Skill | "new" | null
  teams: Array<{ id?: string; name?: string }>
  isAdmin: boolean
  onClose: () => void
  onSaved: () => void
}) {
  const current = skill === "new" || skill === null ? null : skill
  const [name, setName] = useState(current?.name ?? "")
  const [slug, setSlug] = useState(current?.slug ?? "")
  const [description, setDescription] = useState(
    current?.human_description ?? current?.description ?? ""
  )
  const [content, setContent] = useState(() => {
    const bundle = current?.bundle as { content?: string } | undefined
    return bundle?.content ?? ""
  })
  const [teamID, setTeamID] = useState(
    current?.team_id ?? (isAdmin ? "org" : (teams[0]?.id ?? ""))
  )
  const create = $api.useMutation("post", "/v1/skills")
  const update = $api.useMutation("patch", "/v1/skills/{id}")
  if (!skill) return null
  const save = async () => {
    const body = {
      name,
      slug,
      human_description: description,
      description,
      team_id: teamID === "org" ? undefined : teamID,
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
      if (current?.id)
        await update.mutateAsync({ params: { path: { id: current.id } }, body })
      else await create.mutateAsync({ body })
      toast.success(current ? "Skill updated" : "Skill created")
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
              <Modal.Heading>
                {current ? "Edit skill" : "Add skill"}
              </Modal.Heading>
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
              {!current ? (
                <Select
                  aria-label="Scope"
                  value={teamID}
                  onChange={(key) => setTeamID(String(key))}
                >
                  <Select.Trigger>
                    <Select.Value />
                  </Select.Trigger>
                  <Select.Popover>
                    <ListBox>
                      {isAdmin ? (
                        <ListBox.Item id="org">Workspace-wide</ListBox.Item>
                      ) : null}
                      {teams.map((team) =>
                        team.id ? (
                          <ListBox.Item key={team.id} id={team.id}>
                            {team.name ?? "Team"}
                          </ListBox.Item>
                        ) : null
                      )}
                    </ListBox>
                  </Select.Popover>
                </Select>
              ) : null}
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
                isDisabled={
                  !name.trim() || !slug.trim() || !content.trim() || !teamID
                }
                isPending={create.isPending || update.isPending}
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
