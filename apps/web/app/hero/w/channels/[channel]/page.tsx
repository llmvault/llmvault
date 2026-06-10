"use client"

import { useParams } from "next/navigation"
import { useEffect, useMemo, useRef, useState } from "react"
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import ScrollToBottom from "react-scroll-to-bottom"
import { Streamdown } from "streamdown"
import { ChatInput } from "../../_components/chat-input"
import {
  eventRole,
  eventText,
  fetchEmployeeID,
  fetchSessionEvents,
  fetchWebSessions,
  sessionTitle,
  sendSessionMessage,
  streamSessionResponse,
  takePendingStream,
  type WebSession,
} from "../../_lib/sessions"
import { Avatar, Typography } from "@heroui/react"
import { cn } from "@/lib/utils"

type ChatMessage = {
  id: string
  role: "user" | "assistant"
  text: string
  streaming?: boolean
}

export default function ChannelPage() {
  const params = useParams()
  const sessionID = typeof params.channel === "string" ? params.channel : ""
  const queryClient = useQueryClient()
  const abortRef = useRef<AbortController | null>(null)
  const startedPendingRef = useRef(false)
  const [input, setInput] = useState("")
  const [ephemeralMessages, setEphemeralMessages] = useState<ChatMessage[]>([])
  const [streamError, setStreamError] = useState("")
  const [isStreaming, setIsStreaming] = useState(false)

  const employeeQuery = useQuery({
    queryKey: ["web-employee"],
    queryFn: fetchEmployeeID,
  })
  const employeeID = employeeQuery.data ?? ""

  const sessionQuery = useQuery({
    queryKey: ["web-session", employeeID, sessionID],
    enabled: Boolean(employeeID && sessionID),
    queryFn: async () => {
      const page = await fetchWebSessions(employeeID)
      return page.data?.find((session) => session.id === sessionID) ?? null
    },
  })

  const eventsQuery = useInfiniteQuery({
    queryKey: ["web-session-events", employeeID, sessionID],
    enabled: Boolean(employeeID && sessionID),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchSessionEvents(employeeID, sessionID, pageParam),
    getNextPageParam: (page) =>
      page.has_more ? (page.next_cursor ?? undefined) : undefined,
  })

  const persistedMessages = useMemo(() => {
    const newestFirst =
      eventsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
    return [...newestFirst]
      .reverse()
      .map((event): ChatMessage | null => {
        const role = eventRole(event)
        if (!role) return null
        return {
          id: event.id ?? `${event.event_type}-${event.event_at}`,
          role,
          text: eventText(event),
        }
      })
      .filter((message): message is ChatMessage => Boolean(message?.text))
  }, [eventsQuery.data])

  const messages = useMemo(
    () => mergeMessages(persistedMessages, ephemeralMessages),
    [persistedMessages, ephemeralMessages]
  )

  const sendMessage = useMutation({
    mutationFn: (text: string) =>
      sendSessionMessage({ employeeID, sessionID, text }),
    onSuccess: (result, text) => {
      const streamURL = result.response_stream_url
      if (!streamURL) return
      startAssistantStream(streamURL, text)
      queryClient.invalidateQueries({ queryKey: ["web-sessions"] })
    },
    onError: (error) => {
      setStreamError(error.message)
    },
  })

  useEffect(() => {
    return () => abortRef.current?.abort()
  }, [])

  useEffect(() => {
    if (!sessionID || startedPendingRef.current) return
    const pending = takePendingStream(sessionID)
    if (!pending) return
    startedPendingRef.current = true
    startAssistantStream(pending.streamURL, pending.text)
  }, [sessionID])

  function startAssistantStream(streamURL: string, userText: string) {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    setStreamError("")
    setIsStreaming(true)
    const userID = `local-user-${Date.now()}`
    const assistantID = `local-assistant-${Date.now()}`
    setEphemeralMessages((current) => [
      ...current,
      { id: userID, role: "user", text: userText },
      { id: assistantID, role: "assistant", text: "", streaming: true },
    ])

    streamSessionResponse({
      streamURL,
      signal: controller.signal,
      onText: (text, mode) => {
        setEphemeralMessages((current) =>
          current.map((message) =>
            message.id === assistantID
              ? {
                  ...message,
                  text: mode === "replace" ? text : message.text + text,
                }
              : message
          )
        )
      },
    })
      .catch((error) => {
        if (controller.signal.aborted) return
        setStreamError(error instanceof Error ? error.message : "Stream failed")
      })
      .finally(async () => {
        setIsStreaming(false)
        setEphemeralMessages((current) =>
          current.map((message) =>
            message.id === assistantID
              ? { ...message, streaming: false }
              : message
          )
        )
        await queryClient.invalidateQueries({
          queryKey: ["web-session-events", employeeID, sessionID],
        })
        queryClient.invalidateQueries({ queryKey: ["web-sessions"] })
        setTimeout(() => setEphemeralMessages([]), 250)
      })
  }

  const session = sessionQuery.data

  return (
    <div className="relative flex h-full w-full flex-col overflow-hidden">
      <div className="sticky top-0 flex h-14 w-full items-center border-b border-border bg-background px-4">
        <Typography.Paragraph size="sm" weight="medium" className="truncate">
          {session ? sessionTitle(session) : "Hivy"}
        </Typography.Paragraph>
      </div>

      <ScrollToBottom
        className="min-h-0 flex-1 overflow-hidden"
        scrollViewClassName="h-full px-4 py-6"
        initialScrollBehavior="auto"
      >
        <div className="mx-auto flex max-w-4xl flex-col gap-5">
          {employeeQuery.isLoading || eventsQuery.isLoading ? (
            <Typography.Paragraph size="sm" color="muted">
              Loading conversation
            </Typography.Paragraph>
          ) : eventsQuery.isError ? (
            <Typography.Paragraph size="sm" className="text-danger">
              {eventsQuery.error.message}
            </Typography.Paragraph>
          ) : messages.length === 0 ? (
            <Typography.Paragraph size="sm" color="muted">
              Ask Hivy a follow-up.
            </Typography.Paragraph>
          ) : (
            messages.map((message) => (
              <MessageBubble
                key={message.id}
                message={message}
                session={session}
              />
            ))
          )}
          {streamError ? (
            <Typography.Paragraph size="sm" className="text-danger">
              {streamError}
            </Typography.Paragraph>
          ) : null}
          {sendMessage.isError ? (
            <Typography.Paragraph size="sm" className="text-danger">
              {sendMessage.error.message}
            </Typography.Paragraph>
          ) : null}
        </div>
      </ScrollToBottom>

      <div className="sticky bottom-0 shrink-0 overflow-hidden bg-background/95 px-4 pt-2 pb-4">
        <div className="mx-auto max-w-4xl">
          <ChatInput
            compact
            channelName={sessionID}
            value={input}
            onValueChange={setInput}
            onSubmit={(text) => {
              setInput("")
              sendMessage.mutate(text)
            }}
            disabled={!employeeID || isStreaming || sendMessage.isPending}
            isSending={sendMessage.isPending}
            placeholder={isStreaming ? "Hivy is responding" : "Message Hivy"}
          />
        </div>
      </div>
    </div>
  )
}

function MessageBubble({
  message,
  session,
}: {
  message: ChatMessage
  session?: WebSession | null
}) {
  const isUser = message.role === "user"
  return (
    <div
      className={cn(
        "flex w-full flex-col gap-2",
        isUser ? "items-end" : "items-start"
      )}
    >
      {!isUser ? (
        <div className="flex items-center gap-2">
          <Avatar size="sm">
            <Avatar.Fallback>HV</Avatar.Fallback>
          </Avatar>
          <Typography.Paragraph size="sm">
            {session?.source_resource_key ? "Hivy" : "Hivy"}
          </Typography.Paragraph>
        </div>
      ) : null}
      <div
        className={cn(
          "max-w-2xl rounded-3xl px-4 py-3 text-sm leading-6 shadow-sm",
          isUser
            ? "bg-accent text-accent-foreground"
            : "bg-transparent text-foreground shadow-none"
        )}
      >
        {isUser ? (
          <div className="whitespace-pre-wrap">{message.text}</div>
        ) : (
          <Streamdown isAnimating={message.streaming} animated>
            {message.text || (message.streaming ? " " : "")}
          </Streamdown>
        )}
      </div>
    </div>
  )
}

function mergeMessages(
  persistedMessages: ChatMessage[],
  ephemeralMessages: ChatMessage[]
) {
  if (ephemeralMessages.length === 0) return persistedMessages
  const persistedUserTexts = new Set(
    persistedMessages
      .filter((message) => message.role === "user")
      .map((message) => message.text.trim())
  )
  return [
    ...persistedMessages,
    ...ephemeralMessages.filter(
      (message) =>
        message.role !== "user" || !persistedUserTexts.has(message.text.trim())
    ),
  ]
}
