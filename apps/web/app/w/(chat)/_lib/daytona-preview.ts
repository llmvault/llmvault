const DAYTONA_SKIP_PREVIEW_WARNING_HEADER = "X-Daytona-Skip-Preview-Warning"

export function withDaytonaPreviewWarningBypass(
  headers: Record<string, string>
) {
  return {
    ...headers,
    [DAYTONA_SKIP_PREVIEW_WARNING_HEADER]: "true",
  }
}
