"use client"

import { useMemo } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { IntegrationLogo } from "@/components/integration-logo"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useIsAdmin } from "@/lib/auth/use-role"
import type { components } from "@/lib/api/schema"
import {
  deriveProvider,
  providerMeta,
  type Connection,
  type RagSource,
} from "../../knowledge/_lib"
import { ProviderIcon } from "../../knowledge/_provider-icon"
import { enabledIdSet, isProvisioned } from "./_provisioning-lib"
import {
  EmptyProvisioningRow,
  ProvisioningRow,
  ProvisioningSkeleton,
  SectionHeader,
} from "./_provisioning-row"

const TEAM_CONNECTIONS_KEY = [
  "get",
  "/v1/orgs/current/teams/{teamID}/connections",
] as const
const TEAM_SKILLS_KEY = [
  "get",
  "/v1/orgs/current/teams/{teamID}/skills",
] as const
const TEAM_RAG_SOURCES_KEY = [
  "get",
  "/v1/orgs/current/teams/{teamID}/rag-sources",
] as const

export function TeamProvisioningSection({ teamId }: { teamId: string }) {
  const isAdmin = useIsAdmin()
  return (
    <div className="flex flex-col gap-8">
      <TeamConnectionsSection teamId={teamId} readOnly={!isAdmin} />
      <TeamSkillsSection teamId={teamId} readOnly={!isAdmin} />
      {isAdmin ? <TeamKnowledgeSourcesSection teamId={teamId} /> : null}
    </div>
  )
}

type ConnectionRow =
  | components["schemas"]["connectionResponse"]
  | components["schemas"]["databaseConnectionResponse"]
type SkillRow = components["schemas"]["skillResponse"]

function TeamConnectionsSection({
  teamId,
  readOnly,
}: {
  teamId: string
  readOnly: boolean
}) {
  const queryClient = useQueryClient()

  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const databasesQuery = $api.useQuery("get", "/v1/database-integrations")
  const teamConnectionsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/connections",
    { params: { path: { teamID: teamId } } }
  )
  const grantMutation = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/connections"
  )
  const revokeMutation = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{teamID}/connections/{connectionID}"
  )

  const connections = useMemo<ConnectionRow[]>(
    () => [
      ...(connectionsQuery.data?.data ?? []),
      ...(databasesQuery.data ?? []),
    ],
    [connectionsQuery.data?.data, databasesQuery.data]
  )
  const enabledIds = useMemo(
    () => enabledIdSet(teamConnectionsQuery.data?.data),
    [teamConnectionsQuery.data?.data]
  )
  const isBusy = grantMutation.isPending || revokeMutation.isPending
  const isLoading =
    connectionsQuery.isLoading ||
    databasesQuery.isLoading ||
    teamConnectionsQuery.isLoading

  function toggle(connection: ConnectionRow, on: boolean) {
    const id = connection.id
    if (!id) return
    const name =
      connection.name ||
      connection.display_name ||
      connection.provider ||
      "connection"
    const options = {
      onSuccess: () => {
        toast.success(`${on ? "Granted" : "Revoked"} ${name} for this team`)
        queryClient.invalidateQueries({ queryKey: TEAM_CONNECTIONS_KEY })
      },
      onError: (err: unknown) =>
        toast.danger(
          extractErrorMessage(
            err,
            `Could not ${on ? "grant" : "revoke"} connection`
          )
        ),
    }
    if (on) {
      grantMutation.mutate(
        { params: { path: { teamID: teamId } }, body: { connection_id: id } },
        options
      )
    } else {
      revokeMutation.mutate(
        { params: { path: { teamID: teamId, connectionID: id } } },
        options
      )
    }
  }

  const visibleConnections = readOnly
    ? connections.filter((item) => isProvisioned(item.id, enabledIds))
    : connections

  if (readOnly) {
    return (
      <section className="flex flex-col gap-3">
        <SectionHeader
          title="Connections"
          description="Connections this team's agents can use."
        />
        <div className="overflow-hidden rounded-2xl border border-border bg-surface">
          {isLoading ? (
            <ProvisioningSkeleton />
          ) : visibleConnections.length === 0 ? (
            <EmptyProvisioningRow text="No connections are granted to this team yet." />
          ) : (
            visibleConnections.map((connection, index) => (
              <ProvisioningRow
                key={connection.id ?? index}
                readOnly
                last={index === visibleConnections.length - 1}
                icon={
                  <IntegrationLogo
                    provider={connection.provider ?? ""}
                    size={32}
                  />
                }
                title={
                  connection.name ||
                  connection.display_name ||
                  connection.provider ||
                  "Connection"
                }
                subtitle={connection.provider || "Connection"}
                on
                label="Connection is granted to this team"
              />
            ))
          )}
        </div>
      </section>
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Connections"
        description="Granted connections expose their generated MCP tools to every agent in this team."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {isLoading ? (
          <ProvisioningSkeleton />
        ) : connections.length === 0 ? (
          <EmptyProvisioningRow text="No connections exist for this workspace yet." />
        ) : (
          connections.map((connection, index) => {
            const on = isProvisioned(connection.id, enabledIds)
            return (
              <ProvisioningRow
                key={connection.id ?? index}
                last={index === connections.length - 1}
                icon={
                  <IntegrationLogo
                    provider={connection.provider ?? ""}
                    size={32}
                  />
                }
                title={
                  connection.name ||
                  connection.display_name ||
                  connection.provider ||
                  "Connection"
                }
                subtitle={connection.provider || "Connection"}
                on={on}
                disabled={isBusy || !connection.id}
                label={`${on ? "Revoke" : "Grant"} connection for this team`}
                onChange={(selected) => toggle(connection, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}

function TeamSkillsSection({
  teamId,
  readOnly,
}: {
  teamId: string
  readOnly: boolean
}) {
  const queryClient = useQueryClient()
  const skillsQuery = $api.useQuery("get", "/v1/skills")
  const teamSkillsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/skills",
    { params: { path: { teamID: teamId } } }
  )
  const grant = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/skills"
  )
  const revoke = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{teamID}/skills/{skillID}"
  )
  const effective = teamSkillsQuery.data?.data ?? []
  const effectiveIDs = enabledIdSet(effective)
  const skills = readOnly
    ? effective
    : (skillsQuery.data?.skills ?? []).filter(
        (skill: SkillRow) => !skill.team_id
      )
  const toggle = (skill: SkillRow, on: boolean) => {
    if (!skill.id) return
    const options = {
      onSuccess: () =>
        queryClient.invalidateQueries({ queryKey: TEAM_SKILLS_KEY }),
      onError: (error: unknown) =>
        toast.danger(extractErrorMessage(error, "Could not update team skill")),
    }
    if (on)
      grant.mutate(
        { params: { path: { teamID: teamId } }, body: { skill_id: skill.id } },
        options
      )
    else
      revoke.mutate(
        { params: { path: { teamID: teamId, skillID: skill.id } } },
        options
      )
  }
  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Skills"
        description="Org skills explicitly granted to this team. Team-owned skills are available automatically."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {skillsQuery.isLoading || teamSkillsQuery.isLoading ? (
          <ProvisioningSkeleton />
        ) : skills.length === 0 ? (
          <EmptyProvisioningRow text="No skills are available for this team yet." />
        ) : (
          skills.map((skill, index) => {
            const matching = effective.find((item) => item.id === skill.id)
            const on =
              Boolean(matching) || isProvisioned(skill.id, effectiveIDs)
            return (
              <ProvisioningRow
                key={skill.id ?? index}
                last={index === skills.length - 1}
                icon={
                  <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-default">
                    <AppIcon icon="sparkles" className="h-4 w-4" />
                  </span>
                }
                title={skill.name || "Skill"}
                subtitle={skill.slug || "Skill"}
                on={on}
                readOnly={readOnly}
                disabled={grant.isPending || revoke.isPending}
                label={`${on ? "Revoke" : "Grant"} skill`}
                onChange={(selected) => toggle(skill as SkillRow, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}

function TeamKnowledgeSourcesSection({ teamId }: { teamId: string }) {
  const queryClient = useQueryClient()

  const sourcesQuery = $api.useQuery("get", "/v1/rag/sources", {
    params: { query: { page_size: 100 } },
  })
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const teamSourcesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/rag-sources",
    { params: { path: { teamID: teamId } } }
  )
  const grantMutation = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/rag-sources"
  )
  const revokeMutation = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{teamID}/rag-sources/{sourceID}"
  )

  const sources = useMemo(
    () => sourcesQuery.data?.data ?? [],
    [sourcesQuery.data?.data]
  )
  const connectionsById = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const conn of connectionsQuery.data?.data ?? []) {
      if (conn.id) map.set(conn.id, conn)
    }
    return map
  }, [connectionsQuery.data?.data])
  const enabledIds = useMemo(
    () => enabledIdSet(teamSourcesQuery.data?.data),
    [teamSourcesQuery.data?.data]
  )
  const isBusy = grantMutation.isPending || revokeMutation.isPending
  const isLoading =
    sourcesQuery.isLoading ||
    connectionsQuery.isLoading ||
    teamSourcesQuery.isLoading

  function toggle(source: RagSource, on: boolean) {
    const id = source.id
    if (!id) return
    const name = source.name || "source"
    const options = {
      onSuccess: () => {
        toast.success(`${on ? "Granted" : "Revoked"} ${name} for this team`)
        queryClient.invalidateQueries({ queryKey: TEAM_RAG_SOURCES_KEY })
      },
      onError: (err: unknown) =>
        toast.danger(
          extractErrorMessage(
            err,
            `Could not ${on ? "grant" : "revoke"} knowledge source`
          )
        ),
    }
    if (on) {
      grantMutation.mutate(
        { params: { path: { teamID: teamId } }, body: { rag_source_id: id } },
        options
      )
    } else {
      revokeMutation.mutate(
        { params: { path: { teamID: teamId, sourceID: id } } },
        options
      )
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Knowledge sources"
        description="Choose which knowledge sources this team can search."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {isLoading ? (
          <ProvisioningSkeleton />
        ) : sources.length === 0 ? (
          <EmptyProvisioningRow text="No knowledge sources exist for this workspace yet." />
        ) : (
          sources.map((source, index) => {
            const on = isProvisioned(source.id, enabledIds)
            const provider = providerMeta(
              deriveProvider(source, connectionsById)
            )
            return (
              <ProvisioningRow
                key={source.id ?? index}
                last={index === sources.length - 1}
                icon={
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default text-muted">
                    <ProviderIcon icon={provider.icon} className="h-4 w-4" />
                  </span>
                }
                title={source.name || "Untitled source"}
                subtitle={provider.label}
                on={on}
                disabled={isBusy || !source.id}
                label={`${on ? "Revoke" : "Grant"} ${source.name || "source"} for this team`}
                onChange={(selected) => toggle(source, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}
