"use client"

import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowRight01Icon,
  Tick02Icon,
  Loading03Icon,
  RefreshIcon,
} from "@hugeicons/core-free-icons"
import { ModelCombobox } from "@/components/model-combobox"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { EmployeeUpgradeDialog } from "../_components/employee-upgrade-dialog"

export default function SettingsPage() {
  const queryClient = useQueryClient()
  const [upgradeOpen, setUpgradeOpen] = useState(false)
  const [selectedModel, setSelectedModel] = useState<string | null>(null)

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })

  const employee = employeesQuery.data?.data?.[0]
  const employeeID = employee?.id ?? ""
  const canUpgrade = employee?.upgrade_available && employee?.id
  const isUpgrading = employee?.sandbox?.status?.toLowerCase() === "upgrading"
  const modelChanged = !!employee?.model && selectedModel !== employee.model
  const updateModel = $api.useMutation("patch", "/v1/employees/{id}/model")
  const rebootSandbox = $api.useMutation(
    "post",
    "/v1/employees/{id}/sandbox/reboot"
  )

  useEffect(() => {
    setSelectedModel(employee?.model ?? null)
  }, [employee?.model])

  function refreshEmployee() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
    if (employeeID) {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees/{id}"] })
    }
  }

  function saveModel() {
    if (!employeeID || !selectedModel || !modelChanged) return
    updateModel.mutate(
      {
        params: { path: { id: employeeID } },
        body: { model: selectedModel } as never,
      },
      {
        onSuccess: (data) => {
          setSelectedModel(data.employee?.model ?? selectedModel)
          toast.success("Employee model saved and synced")
          refreshEmployee()
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Failed to save employee model")
          )
          refreshEmployee()
        },
      }
    )
  }

  function rebootEmployee() {
    if (!employeeID) return
    rebootSandbox.mutate(
      {
        params: { path: { id: employeeID } },
      },
      {
        onSuccess: () => {
          toast.success("Employee sandbox rebooted and synced")
          refreshEmployee()
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Failed to reboot employee sandbox")
          )
          refreshEmployee()
        },
      }
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-7">
      <div className="max-w-2xl">
        <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Settings
        </h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          Manage workspace preferences and employee runtime behavior.
        </p>
      </div>

      <Tabs defaultValue="general" className="gap-6">
        <TabsList className="w-fit">
          <TabsTrigger value="general">General</TabsTrigger>
        </TabsList>

        <TabsContent value="general" className="flex flex-col gap-6">
          <div className="flex flex-col gap-6 rounded-2xl border border-border bg-card p-6">
            <div>
              <h2 className="font-heading text-lg font-medium text-foreground">
                Employee model
              </h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                Choose the model Hivy uses for workspace conversations.
              </p>
            </div>

            <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
              <EmployeeModelsCombobox
                value={selectedModel}
                onSelect={setSelectedModel}
                disabled={updateModel.isPending || isUpgrading}
              />
              <Button
                type="button"
                disabled={
                  !employeeID ||
                  !selectedModel ||
                  !modelChanged ||
                  updateModel.isPending ||
                  isUpgrading
                }
                onClick={saveModel}
                className="w-full lg:w-auto h-14"
              >
                {updateModel.isPending ? "Saving..." : "Save changes"}
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-6 rounded-2xl border border-border bg-card p-6">
            <div>
              <h2 className="font-heading text-lg font-medium text-foreground">
                Sandbox runtime
              </h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                Your employee sandbox runs on a dedicated runtime image.
                Upgrades recreate the sandbox with the newest image while
                preserving the runtime database.
              </p>
            </div>

            <div className="flex items-center gap-3 rounded-xl border border-border bg-muted/30 p-4">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-muted">
                {isUpgrading ? (
                  <HugeiconsIcon
                    icon={Loading03Icon}
                    className="size-5 animate-spin text-primary"
                  />
                ) : canUpgrade ? (
                  <HugeiconsIcon
                    icon={Alert02Icon}
                    className="size-5 text-primary"
                  />
                ) : (
                  <HugeiconsIcon
                    icon={Tick02Icon}
                    className="size-5 text-success"
                  />
                )}
              </div>
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <p className="text-sm font-medium text-foreground">
                  {isUpgrading
                    ? "Upgrade in progress"
                    : canUpgrade
                      ? "Upgrade available"
                      : "Sandbox is up to date"}
                </p>
                <p className="text-xs text-muted-foreground">
                  {isUpgrading
                    ? "Your sandbox is being upgraded. This may take a few minutes."
                    : canUpgrade
                      ? "A newer runtime image is available for your sandbox."
                      : "Your sandbox is running the latest runtime image."}
                </p>
              </div>
              {canUpgrade ? (
                <Button
                  size="sm"
                  onClick={() => setUpgradeOpen(true)}
                  className="shrink-0 gap-1.5"
                >
                  Upgrade
                  <HugeiconsIcon icon={ArrowRight01Icon} className="size-3.5" />
                </Button>
              ) : null}
            </div>

            <div className="flex flex-col gap-4 border-t border-border pt-5 sm:flex-row sm:items-center sm:justify-between">
              <div className="max-w-xl">
                <p className="text-sm font-medium text-foreground">
                  Reboot employee
                </p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  Restart the sandbox, mint fresh proxy credentials, push the
                  full runtime config, and verify readiness.
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={rebootEmployee}
                disabled={!employeeID || isUpgrading || updateModel.isPending}
                loading={rebootSandbox.isPending}
                className="w-full gap-1.5 sm:w-auto"
              >
                <HugeiconsIcon icon={RefreshIcon} className="size-3.5" />
                Reboot
              </Button>
            </div>
          </div>
        </TabsContent>
      </Tabs>

      {employee ? (
        <EmployeeUpgradeDialog
          employee={employee}
          open={upgradeOpen}
          onOpenChange={setUpgradeOpen}
        />
      ) : null}
    </div>
  )
}

function EmployeeModelsCombobox({
  value,
  onSelect,
  disabled,
}: {
  value?: string | null
  onSelect?: (model: string) => void
  disabled?: boolean
}) {
  const { data, isLoading } = $api.useQuery("get", "/v1/employees/models", {})

  return (
    <ModelCombobox
      models={data ?? []}
      value={value}
      onSelect={onSelect}
      loading={isLoading}
      disabled={isLoading || disabled}
    />
  )
}
