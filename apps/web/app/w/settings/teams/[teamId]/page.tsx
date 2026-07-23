import { redirect } from "next/navigation"

export default async function TeamSettingsRedirect({
  params,
}: {
  params: Promise<{ teamId: string }>
}) {
  const { teamId } = await params
  redirect(`/w/teams/${teamId}`)
}
