import { ChatCanvas } from "@/app/w/(chat)/_components/chat-canvas"

export default async function ChannelSessionPage({
  params,
}: {
  params: Promise<{ channel: string; session: string }>
}) {
  const { channel, session } = await params
  return <ChatCanvas channelSlug={channel} sessionId={session} />
}
