import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"

interface AppLaunchResult {
  /**
   * The app's own `/auth/callback?token=…` URL — load this in the iframe. The
   * app verifies the one-time token, sets its own session cookie, and serves
   * the SPA with the user already authenticated.
   */
  iframeUrl: string
  status: string
}

/**
 * Exchange a one-time launch token for an app and build the iframe URL that
 * boots it authenticated. The browser only ever talks to the app: we mount
 * `{base}/auth/callback?token=…`, where `base` is the deployed app URL when
 * running, else the builder-sandbox preview URL.
 *
 * The token is one-time and short-lived (60s), so this must be called fresh
 * every time an app is opened — never cache the result.
 */
export async function launchAppPreview(appId: string): Promise<AppLaunchResult> {
  const { data, error } = await api.GET("/v1/apps/{appID}/launch", {
    params: { path: { appID: appId } },
  })
  if (error !== undefined || !data) {
    throw new Error(extractErrorMessage(error, "Could not open app"))
  }
  const base = data.app_url || data.preview_url
  if (!base || !data.token) {
    throw new Error("This app isn't ready to preview yet.")
  }
  const callback = new URL("/auth/callback", base)
  callback.searchParams.set("token", data.token)
  return { iframeUrl: callback.toString(), status: data.status ?? "" }
}
