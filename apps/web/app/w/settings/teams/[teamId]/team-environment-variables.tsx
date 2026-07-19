"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import {
  AddVariableForm,
  EnvironmentVariableRow,
  EnvVarsSkeleton,
} from "./_environment-variable-forms"

/**
 * Team environment-variable settings. Values are
 * write-only: the API never returns them once saved, so the list only shows
 * masked placeholders. The server remains the source of truth for team access.
 */
export function TeamEnvironmentVariablesPanel({ teamId }: { teamId: string }) {
  const queryClient = useQueryClient()
  const varsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{id}/environment-variables",
    { params: { path: { id: teamId } } },
    { enabled: Boolean(teamId), retry: false }
  )
  const createVar = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{id}/environment-variables"
  )
  const updateVar = $api.useMutation(
    "patch",
    "/v1/orgs/current/teams/{id}/environment-variables/{name}"
  )
  const deleteVar = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{id}/environment-variables/{name}"
  )

  const [adding, setAdding] = useState(false)
  const [editingName, setEditingName] = useState<string | null>(null)
  const [deletingName, setDeletingName] = useState<string | null>(null)

  const vars = varsQuery.data?.data ?? []

  function invalidateList() {
    queryClient.invalidateQueries({
      queryKey: queryKeys.teamEnvironmentVariables(),
    })
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-sm font-medium text-foreground">
            Environment variables
          </h2>
          <p className="text-muted-foreground mt-1 text-sm">
            Available to sessions started for this team. Values
            are encrypted and hidden once saved.
          </p>
        </div>
        {!adding ? (
          <Button
            variant="tertiary"
            size="sm"
            className="shrink-0"
            isDisabled={!teamId}
            onPress={() => {
              setEditingName(null)
              setAdding(true)
            }}
          >
            <AppIcon icon="plus" className="h-4 w-4" />
            Add
          </Button>
        ) : null}
      </div>

      {adding ? (
        <AddVariableForm
          pending={createVar.isPending}
          submitError={
            createVar.isError
              ? extractErrorMessage(
                  createVar.error,
                  "Could not add environment variable"
                )
              : null
          }
          onCancel={() => {
            createVar.reset()
            setAdding(false)
          }}
          onSubmit={(name, value, description) => {
            createVar.mutate(
              {
                params: { path: { id: teamId } },
                body: { name, value, description },
              },
              {
                onSuccess: () => {
                  invalidateList()
                  toast.success("Environment variable added")
                  setAdding(false)
                },
              }
            )
          }}
        />
      ) : null}

      {varsQuery.isLoading ? <EnvVarsSkeleton /> : null}

      {varsQuery.isError ? (
        <div className="rounded-xl border border-border bg-surface px-5 py-4 text-sm text-danger">
          Could not load environment variables.
        </div>
      ) : null}

      {!varsQuery.isLoading &&
      !varsQuery.isError &&
      vars.length === 0 &&
      !adding ? (
        <div className="bg-card flex min-h-40 flex-col items-center justify-center rounded-xl px-6 text-center">
          <AppIcon icon="braces" className="text-muted-foreground h-7 w-7" />
          <p className="mt-3 text-sm font-medium text-foreground">
            No environment variables yet
          </p>
          <p className="text-muted-foreground mt-1 max-w-sm text-sm">
            Add variables for sessions started for this team.
          </p>
        </div>
      ) : null}

      {vars.length ? (
        <div className="flex flex-col gap-2">
          {vars.map((item) => {
            const name = item.name ?? ""
            if (!name) return null
            return (
              <EnvironmentVariableRow
                key={name}
                name={name}
                description={item.description ?? ""}
                editing={editingName === name}
                deleting={deletingName === name}
                updatePending={updateVar.isPending}
                deletePending={deleteVar.isPending}
                updateError={
                  updateVar.isError && editingName === name
                    ? extractErrorMessage(
                        updateVar.error,
                        "Could not update environment variable"
                      )
                    : null
                }
                onEdit={() => {
                  setAdding(false)
                  updateVar.reset()
                  setEditingName(name)
                }}
                onCancelEdit={() => {
                  updateVar.reset()
                  setEditingName(null)
                }}
                onSubmitEdit={(newName, value, description) => {
                  const body: {
                    name?: string
                    value?: string
                    description?: string
                  } = {}
                  if (newName !== name) body.name = newName
                  if (value) body.value = value
                  if (description !== (item.description ?? "")) {
                    body.description = description
                  }
                  if (Object.keys(body).length === 0) {
                    setEditingName(null)
                    return
                  }
                  updateVar.mutate(
                    {
                      params: { path: { id: teamId, name } },
                      body,
                    },
                    {
                      onSuccess: () => {
                        invalidateList()
                        toast.success("Environment variable updated")
                        setEditingName(null)
                      },
                    }
                  )
                }}
                onDelete={() => setDeletingName(name)}
                onCancelDelete={() => setDeletingName(null)}
                onConfirmDelete={() => {
                  deleteVar.mutate(
                    { params: { path: { id: teamId, name } } },
                    {
                      onSuccess: () => {
                        invalidateList()
                        toast.success("Environment variable deleted")
                        setDeletingName(null)
                      },
                      onError: (error) =>
                        toast.danger(
                          extractErrorMessage(
                            error,
                            "Could not delete environment variable"
                          )
                        ),
                    }
                  )
                }}
              />
            )
          })}
        </div>
      ) : null}
    </div>
  )
}
