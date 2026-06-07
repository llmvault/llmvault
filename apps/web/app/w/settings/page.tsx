"use client"

import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Alert02Icon,
  ArrowRight01Icon,
  Delete02Icon,
  PencilIcon,
  Tick02Icon,
  Loading03Icon,
  RefreshIcon,
} from "@hugeicons/core-free-icons"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { ModelCombobox } from "@/components/model-combobox"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { EmployeeUpgradeDialog } from "../_components/employee-upgrade-dialog"

type OrgEnvironmentVariable = {
  name: string
  env_key: string
}

const environmentNamePattern = /^[A-Z_][A-Z0-9_]*$/

export default function SettingsPage() {
  const queryClient = useQueryClient()
  const [upgradeOpen, setUpgradeOpen] = useState(false)
  const [selectedModel, setSelectedModel] = useState<string | null>(null)
  const [envDialogOpen, setEnvDialogOpen] = useState(false)
  const [editingEnv, setEditingEnv] = useState<OrgEnvironmentVariable | null>(
    null
  )
  const [envName, setEnvName] = useState("")
  const [envValue, setEnvValue] = useState("")
  const [deletingEnv, setDeletingEnv] = useState<OrgEnvironmentVariable | null>(
    null
  )

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })
  const environmentVariablesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/environment-variables",
    {}
  )

  const employee = employeesQuery.data?.data?.[0]
  const environmentVariables =
    (environmentVariablesQuery.data?.data as OrgEnvironmentVariable[] | undefined) ??
    []
  const employeeID = employee?.id ?? ""
  const canUpgrade = employee?.upgrade_available && employee?.id
  const isUpgrading = employee?.sandbox?.status?.toLowerCase() === "upgrading"
  const modelChanged = !!employee?.model && selectedModel !== employee.model
  const updateModel = $api.useMutation("patch", "/v1/employees/{id}/model")
  const rebootSandbox = $api.useMutation(
    "post",
    "/v1/employees/{id}/sandbox/reboot"
  )
  const createEnvVar = $api.useMutation(
    "post",
    "/v1/orgs/current/environment-variables"
  )
  const updateEnvVar = $api.useMutation(
    "patch",
    "/v1/orgs/current/environment-variables/{name}"
  )
  const deleteEnvVar = $api.useMutation(
    "delete",
    "/v1/orgs/current/environment-variables/{name}"
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

  function refreshEnvironmentVariables() {
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/orgs/current/environment-variables"],
    })
    refreshEmployee()
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

  function openCreateEnvironmentVariable() {
    setEditingEnv(null)
    setEnvName("")
    setEnvValue("")
    setEnvDialogOpen(true)
  }

  function openEditEnvironmentVariable(envVar: OrgEnvironmentVariable) {
    setEditingEnv(envVar)
    setEnvName(envVar.name)
    setEnvValue("")
    setEnvDialogOpen(true)
  }

  function closeEnvironmentVariableDialog() {
    setEnvDialogOpen(false)
    setEditingEnv(null)
    setEnvName("")
    setEnvValue("")
  }

  function normalizedEnvironmentName() {
    return envName.trim().toUpperCase()
  }

  function validateEnvironmentName(name: string) {
    if (!name) return "Name is required"
    if (name.startsWith("HIVY_ORG_")) {
      return "Remove the HIVY_ORG_ prefix from the name"
    }
    if (!environmentNamePattern.test(name)) {
      return "Use letters, numbers, and underscores. Start with a letter or underscore."
    }
    return null
  }

  function saveEnvironmentVariable() {
    const name = normalizedEnvironmentName()
    const nameError = validateEnvironmentName(name)
    if (nameError) {
      toast.error(nameError)
      return
    }

    if (!editingEnv) {
      if (!envValue) {
        toast.error("Value is required")
        return
      }
      createEnvVar.mutate(
        {
          body: { name, value: envValue } as never,
        },
        {
          onSuccess: () => {
            toast.success("Environment variable saved and synced")
            closeEnvironmentVariableDialog()
            refreshEnvironmentVariables()
          },
          onError: (error) => {
            toast.error(
              extractErrorMessage(
                error,
                "Failed to save environment variable"
              )
            )
            refreshEnvironmentVariables()
          },
        }
      )
      return
    }

    const body: { name?: string; value?: string } = {}
    if (name !== editingEnv.name) body.name = name
    if (envValue) body.value = envValue
    if (!body.name && !body.value) return

    updateEnvVar.mutate(
      {
        params: { path: { name: editingEnv.name } },
        body: body as never,
      },
      {
        onSuccess: () => {
          toast.success("Environment variable updated and synced")
          closeEnvironmentVariableDialog()
          refreshEnvironmentVariables()
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Failed to update environment variable")
          )
          refreshEnvironmentVariables()
        },
      }
    )
  }

  function deleteEnvironmentVariable() {
    if (!deletingEnv) return
    deleteEnvVar.mutate(
      {
        params: { path: { name: deletingEnv.name } },
      },
      {
        onSuccess: () => {
          toast.success("Environment variable deleted and synced")
          setDeletingEnv(null)
          refreshEnvironmentVariables()
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Failed to delete environment variable")
          )
          refreshEnvironmentVariables()
        },
      }
    )
  }

  const normalizedEnvName = normalizedEnvironmentName()
  const canSaveEnvironmentVariable =
    !createEnvVar.isPending &&
    !updateEnvVar.isPending &&
    normalizedEnvName.length > 0 &&
    (editingEnv
      ? normalizedEnvName !== editingEnv.name || envValue.length > 0
      : envValue.length > 0)

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
              <h2 className="font-sans text-lg font-medium text-foreground">
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
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 className="font-sans text-lg font-medium text-foreground">
                  Environment variables
                </h2>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                  Custom runtime variables for employee and specialist
                  sandboxes.
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                onClick={openCreateEnvironmentVariable}
                className="w-full gap-1.5 sm:w-auto"
              >
                <HugeiconsIcon icon={Add01Icon} className="size-3.5" />
                Add variable
              </Button>
            </div>

            {environmentVariablesQuery.isLoading ? (
              <div className="flex h-28 items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground">
                Loading variables...
              </div>
            ) : environmentVariables.length === 0 ? (
              <div className="flex h-28 items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground">
                No environment variables
              </div>
            ) : (
              <div className="rounded-md border border-border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>Runtime key</TableHead>
                      <TableHead className="w-24 text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {environmentVariables.map((envVar) => (
                      <TableRow key={envVar.name}>
                        <TableCell className="font-mono text-xs text-foreground">
                          {envVar.name}
                        </TableCell>
                        <TableCell>
                          <span className="inline-flex rounded-md bg-muted px-2 py-1 font-mono text-xs text-muted-foreground">
                            {envVar.env_key}
                          </span>
                        </TableCell>
                        <TableCell>
                          <div className="flex justify-end gap-1">
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-sm"
                              aria-label={`Edit ${envVar.name}`}
                              title={`Edit ${envVar.name}`}
                              onClick={() => openEditEnvironmentVariable(envVar)}
                            >
                              <HugeiconsIcon
                                icon={PencilIcon}
                                className="size-4"
                              />
                            </Button>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-sm"
                              aria-label={`Delete ${envVar.name}`}
                              title={`Delete ${envVar.name}`}
                              onClick={() => setDeletingEnv(envVar)}
                            >
                              <HugeiconsIcon
                                icon={Delete02Icon}
                                className="size-4 text-destructive"
                              />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>

          <div className="flex flex-col gap-6 rounded-2xl border border-border bg-card p-6">
            <div>
              <h2 className="font-sans text-lg font-medium text-foreground">
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

      <Dialog
        open={envDialogOpen}
        onOpenChange={(open) => {
          if (!open) closeEnvironmentVariableDialog()
          else setEnvDialogOpen(open)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <form
            className="flex flex-col gap-5"
            onSubmit={(event) => {
              event.preventDefault()
              saveEnvironmentVariable()
            }}
          >
            <DialogHeader>
              <DialogTitle>
                {editingEnv ? "Edit variable" : "Add variable"}
              </DialogTitle>
              <DialogDescription>
                Values are encrypted and hidden after saving.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-4">
              <div className="flex flex-col gap-2">
                <Label htmlFor="environment-name">Name</Label>
                <Input
                  id="environment-name"
                  value={envName}
                  onChange={(event) => setEnvName(event.target.value)}
                  placeholder="STRIPE_API_KEY"
                  autoComplete="off"
                  autoFocus
                />
              </div>

              <div className="flex flex-col gap-2">
                <Label htmlFor="environment-value">
                  {editingEnv ? "New value" : "Value"}
                </Label>
                <Input
                  id="environment-value"
                  type="password"
                  value={envValue}
                  onChange={(event) => setEnvValue(event.target.value)}
                  placeholder={
                    editingEnv ? "Leave blank to keep current value" : "Secret"
                  }
                  autoComplete="off"
                />
              </div>

              <div className="rounded-md border border-border bg-muted/30 px-3 py-2">
                <p className="text-xs text-muted-foreground">Runtime key</p>
                <p className="mt-1 break-all font-mono text-xs text-foreground">
                  {normalizedEnvName
                    ? `HIVY_ORG_${normalizedEnvName}`
                    : "HIVY_ORG_NAME"}
                </p>
              </div>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={closeEnvironmentVariableDialog}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={!canSaveEnvironmentVariable}
                loading={createEnvVar.isPending || updateEnvVar.isPending}
              >
                {editingEnv ? "Save changes" : "Add variable"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deletingEnv !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingEnv(null)
        }}
        title="Delete variable"
        description={`Delete ${deletingEnv?.name ?? "this variable"} from employee and specialist sandbox runtime config?`}
        confirmLabel="Delete"
        destructive
        loading={deleteEnvVar.isPending}
        onConfirm={deleteEnvironmentVariable}
      />
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
