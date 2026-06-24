export async function readMicrophonePermission() {
  if (!navigator.permissions?.query) return null

  try {
    return await navigator.permissions.query({
      name: "microphone" as PermissionName,
    })
  } catch {
    return null
  }
}
