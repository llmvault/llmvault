import { ChatCanvas } from "@/app/w/(chat)/_components/chat-canvas"

export default async function ChannelPage({
  params,
}: {
  params: Promise<{ channel: string }>
}) {
  const { channel } = await params
  return <ChatCanvas channelSlug={channel} />
}
