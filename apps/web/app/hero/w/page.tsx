"use client"

import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { ChatInput } from "./_components/chat-input"
import {
  fetchEmployeeID,
  savePendingStream,
  sendSessionMessage,
} from "./_lib/sessions"

export default function DashboardPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [message, setMessage] = useState("")

  const createSession = useMutation({
    mutationFn: async (text: string) => {
      const employeeID = await fetchEmployeeID()
      return sendSessionMessage({ employeeID, text })
    },
    onSuccess: (result, text) => {
      const sessionID = result.employee_session_id
      const streamURL = result.response_stream_url
      if (!sessionID || !streamURL) return
      savePendingStream({
        sessionID,
        text,
        streamURL,
        createdAt: Date.now(),
      })
      queryClient.invalidateQueries({ queryKey: ["web-sessions"] })
      router.push(`/hero/w/channels/${sessionID}`)
    },
  })

  return (
    <div className="flex h-screen w-full flex-1 items-center p-2">
      <div className="mx-auto w-full max-w-4xl">
        {createSession.isError ? (
          <p className="text-danger mb-3 text-sm">
            {createSession.error.message}
          </p>
        ) : null}
        <ChatInput
          compact
          value={message}
          onValueChange={setMessage}
          onSubmit={(text) => createSession.mutate(text)}
          disabled={createSession.isPending}
          isSending={createSession.isPending}
        />
      </div>
    </div>
  )
}
