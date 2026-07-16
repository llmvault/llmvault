"use client"

import { Modal, useOverlayState } from "@heroui/react"
import type { components } from "@/lib/api/schema"
import {
  DatabaseConnectionModalContent,
  isDatabaseProvider,
} from "@/app/w/(chat)/plugins/database-connection-modal-content"
import { IntegrationCredentialsForm } from "@/app/w/(chat)/plugins/integration-credentials-form"
import type { AvailableIntegration } from "@/app/w/(chat)/plugins/integration-auth"
import type { ConnectOptions } from "@/app/w/(chat)/plugins/use-connect-integration"
import type { PluginRequirement } from "@/app/w/(chat)/plugins/_lib"

type DatabaseConnection = components["schemas"]["databaseConnectionResponse"]

export type ConnectionModalState = {
  view: "integration" | "database"
  requirement: PluginRequirement
}

export function RequiredConnectionModal({
  modal,
  state,
  integration,
  isPending,
  canManage,
  onBack,
  onIntegrationConnect,
  onDatabaseConnected,
}: {
  modal: ConnectionModalState | null
  state: ReturnType<typeof useOverlayState>
  integration: AvailableIntegration | undefined
  isPending: boolean
  canManage: boolean
  onBack: () => void
  onIntegrationConnect: (options?: ConnectOptions) => void
  onDatabaseConnected: (connection: DatabaseConnection) => void
}) {
  const requirement = modal?.requirement
  const provider = requirement?.provider

  return (
    <Modal.Root state={state}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog
            className={`relative w-full bg-background p-0 shadow-xl outline-none ${
              modal?.view === "database" ? "max-w-3xl" : "max-w-sm"
            }`}
          >
            {modal?.view === "database" && isDatabaseProvider(provider) ? (
              <DatabaseConnectionModalContent
                provider={provider}
                canManage={canManage}
                onConnected={onDatabaseConnected}
              />
            ) : modal?.view === "integration" && integration ? (
              <IntegrationCredentialsForm
                integration={integration}
                isSubmitting={isPending}
                onBack={onBack}
                onSubmit={onIntegrationConnect}
              />
            ) : null}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal.Root>
  )
}
