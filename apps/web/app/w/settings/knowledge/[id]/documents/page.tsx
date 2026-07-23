import { redirect } from "next/navigation"

export default async function KnowledgeDocumentsSettingsRedirect({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params
  redirect(`/w/knowledge/${id}/documents`)
}
