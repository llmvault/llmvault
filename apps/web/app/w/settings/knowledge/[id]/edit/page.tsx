import { redirect } from "next/navigation"

export default async function KnowledgeEditSettingsRedirect({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params
  redirect(`/w/knowledge/${id}/edit`)
}
