export function extractErrorMessage(
  error: unknown,
  fallback: string,
  context?: string
): string {
  const message = errorMessageValue(error)
  if (!message) return fallback
  return context ? `${context}: ${message}` : message
}

function errorMessageValue(error: unknown): string | null {
  if (error instanceof Error && error.message.trim()) {
    return error.message
  }
  if (typeof error === "string" && error.trim()) {
    return error
  }
  if (!isRecord(error)) return null

  return (
    stringValue(error.error) ??
    stringValue(error.message) ??
    stringValue(error.error_message) ??
    errorMessageValue(error.error)
  )
}

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value.trim() ? value : null
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}
