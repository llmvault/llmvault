package runtimeevents

const (
	EventTurnStarted       = "turn_started"
	EventToken             = "token"
	EventThinking          = "thinking"
	EventToolResult        = "tool_result"
	EventFinal             = "final"
	EventTurnFailed        = "turn_failed"
	EventTurnInterrupted   = "turn_interrupted"
	EventQuestionRequested = "question_requested"
	EventQuestionAnswered  = "question_answered"
	EventPlanUpdated       = "plan_updated"
	EventSubagentStarted   = "subagent_started"
	EventSubagentCompleted = "subagent_completed"
	EventSubagentErrored   = "subagent_errored"
	EventModelUsage        = "model_usage"
	EventError             = "error"
	EventSessionWaiting    = "session_waiting"

	EventAgentError              = "agent_error"
	EventConversationCreated     = "conversation_created"
	EventConversationEnded       = "conversation_ended"
	EventDone                    = "done"
	EventMessageReceived         = "message_received"
	EventUserMessageReceived     = "user.message.received"
	EventReasoningDelta          = "reasoning_delta"
	EventResponseChunk           = "response_chunk"
	EventResponseCompleted       = "response_completed"
	EventResponseStarted         = "response_started"
	EventTodoUpdated             = "todo_updated"
	EventToolApprovalRequired    = "tool_approval_required"
	EventToolApprovalResolved    = "tool_approval_resolved"
	EventToolCallCompleted       = "tool_call_completed"
	EventToolCallStarted         = "tool_call_started"
	EventTurnCompleted           = "turn_completed"
	EventBackgroundTaskCompleted = "background_task_completed"
	EventReasoningStarted        = "reasoning_started"
	EventReasoningCompleted      = "reasoning_completed"
)

func IsTerminalEventType(eventType string) bool {
	switch eventType {
	case EventAgentError, EventConversationEnded, EventDone, EventTurnCompleted, EventTurnFailed, EventTurnInterrupted:
		return true
	default:
		return false
	}
}

func ReleasesSessionTurn(eventType string) bool {
	switch eventType {
	case EventTurnCompleted, EventTurnFailed, EventTurnInterrupted:
		return true
	default:
		return false
	}
}
