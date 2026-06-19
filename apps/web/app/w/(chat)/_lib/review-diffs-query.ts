import type { RuntimeSandboxAccess } from "@/app/w/(chat)/_lib/runtime-repos"

export function reviewDiffsQueryKey(
  sessionId?: string,
  sandboxAccess?: Pick<RuntimeSandboxAccess, "sandbox_base_url" | "token">
) {
  return [
    "sandbox-runtime-review-diffs",
    sessionId,
    sandboxAccess?.sandbox_base_url,
    sandboxAccess?.token,
  ] as const
}
