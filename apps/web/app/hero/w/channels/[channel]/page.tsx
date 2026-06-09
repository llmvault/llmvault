"use client"

import { useParams } from "next/navigation"
import { ChatInput } from "../../_components/chat-input"
import { Avatar, Typography } from "@heroui/react"

export default function ChannelPage() {
  const params = useParams()
  const channel = typeof params.channel === "string" ? params.channel : ""

  return (
    <div className="relative flex h-full w-full flex-col overflow-hidden">
      <div className="sticky top-0 h-14 w-full border-b border-border pt-2"></div>
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-4xl p-4">
          <div className="mt-8 flex flex-col gap-4">
            <div className="flex items-center gap-2 self-end">
              <Avatar size="sm">
                <Avatar.Fallback>HV</Avatar.Fallback>
              </Avatar>

              <Typography.Paragraph>Hivy safaree</Typography.Paragraph>
            </div>
            <div className="flex w-full flex-col gap-3">
              <div className="flex w-full justify-end">
                <div className="w-auto max-w-2xl rounded-3xl bg-accent-foreground p-2 shadow-sm">
                  Can you make sure this works on AWS also?
                </div>
              </div>
            </div>

            <div className="flex w-full flex-col gap-3">
              <div className="flex items-center gap-2">
                <Avatar size="sm">
                  <Avatar.Fallback>HV</Avatar.Fallback>
                </Avatar>

                <Typography.Paragraph>Hivy safaree</Typography.Paragraph>
              </div>

              <div className="flex w-full justify-start">
                <div className="flex w-auto max-w-2xl flex-col gap-3 rounded-3xl p-2">
                  <p>
                    Lorem ipsum dolor sit, amet consectetur adipisicing elit.
                    Quisquam ducimus a eveniet. Enim officiis omnis alias
                    dolorem, nisi ex molestias commodi hic voluptas quod
                    nostrum. Sit perspiciatis asperiores quis pariatur.
                  </p>
                  <pre>
                    <code>const user = ['1', '2', '3']</code>
                  </pre>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="sticky bottom-0 h-48 shrink-0 overflow-hidden">
        <div className="mx-auto max-w-4xl">
          <ChatInput channelName={channel} />
        </div>
      </div>
    </div>
  )
}
