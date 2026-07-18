import { ChatCanvas } from "@/app/w/(chat)/_components/chat-canvas"

export default async function SessionPage({
  params,
}: {
  params: Promise<{ session: string }>
}) {
  const { session } = await params
  return <ChatCanvas sessionId={session} />
}
