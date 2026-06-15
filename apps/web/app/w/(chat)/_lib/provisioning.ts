import type { components } from "@/lib/api/schema"

export type WorkspaceAgent = components["schemas"]["agentListItem"]

export type WorkspaceProvisioningPhase =
  | "loading"
  | "ready"
  | "provisioning"
  | "retryable"

export interface WorkspaceProvisioningState {
  phase: WorkspaceProvisioningPhase
  defaultAgent?: WorkspaceAgent
  sandboxStatus?: string
  message: string
  shouldPoll: boolean
}

interface ResolveWorkspaceProvisioningStateInput {
  authLoading: boolean
  orgLoading: boolean
  agentsLoading: boolean
  orgError: boolean
  agentsError: boolean
  agents: WorkspaceAgent[]
}

const provisioningStatuses = new Set(["creating", "starting", "upgrading"])

export function findDefaultWorkspaceAgent(
  agents: WorkspaceAgent[]
): WorkspaceAgent | undefined {
  return (
    agents.find((agent) => agent.is_default) ??
    agents.find((agent) => agent.name?.toLowerCase() === "hivy")
  )
}

export function resolveWorkspaceProvisioningState({
  authLoading,
  orgLoading,
  agentsLoading,
  orgError,
  agentsError,
  agents,
}: ResolveWorkspaceProvisioningStateInput): WorkspaceProvisioningState {
  if (authLoading || orgLoading || agentsLoading) {
    return {
      phase: "loading",
      message: "Loading workspace status",
      shouldPoll: false,
    }
  }

  if (orgError || agentsError) {
    return {
      phase: "retryable",
      message: "Workspace status could not be loaded.",
      shouldPoll: false,
    }
  }

  const defaultAgent = findDefaultWorkspaceAgent(agents)
  if (!defaultAgent) {
    return {
      phase: "provisioning",
      message: "Waiting for the Hivy agent to be created.",
      shouldPoll: true,
    }
  }

  const sandboxStatus = defaultAgent.sandbox?.status
  if (sandboxStatus === "running") {
    return {
      phase: "ready",
      defaultAgent,
      sandboxStatus,
      message: "Workspace is ready.",
      shouldPoll: false,
    }
  }

  if (!sandboxStatus || provisioningStatuses.has(sandboxStatus)) {
    return {
      phase: "provisioning",
      defaultAgent,
      sandboxStatus,
      message: "Preparing the Hivy agent sandbox.",
      shouldPoll: true,
    }
  }

  return {
    phase: "retryable",
    defaultAgent,
    sandboxStatus,
    message: `The Hivy agent sandbox is ${sandboxStatus}.`,
    shouldPoll: false,
  }
}
