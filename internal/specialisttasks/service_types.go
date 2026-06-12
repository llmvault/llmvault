package specialisttasks

import (
	"time"

	"github.com/usehivy/hivy/internal/model"
)

type LaunchRequest struct {
	Token             *model.Token
	SpecialistSlug    string
	Brief             string
	Metadata          map[string]any
	EmployeeSessionID string
}

type LaunchResponse struct {
	TaskID            string `json:"task_id"`
	SpecialistSlug    string `json:"specialist_slug"`
	EmployeeSessionID string `json:"employee_session_id"`
	SandboxID         string `json:"sandbox_id"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	SystemReminder    string `json:"system_reminder"`
	NextAction        string `json:"next_action"`
}

type AvailableSpecialistsResponse struct {
	Specialists []AvailableSpecialist `json:"specialists"`
	Count       int                   `json:"count"`
	NextAction  string                `json:"next_action"`
}

type AvailableSpecialist struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Version     int    `json:"version"`
}

type TaskStatusResponse struct {
	TaskID          string     `json:"task_id"`
	SpecialistSlug  string     `json:"specialist_slug"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	LastActivityAt  *time.Time `json:"last_activity_at,omitempty"`
	ActivitySummary string     `json:"activity_summary,omitempty"`
	LatestMessage   string     `json:"latest_message,omitempty"`
	LatestError     string     `json:"latest_error,omitempty"`
	NextAction      string     `json:"next_action"`
}

type TaskTimelineResponse struct {
	TaskID         string                    `json:"task_id"`
	SpecialistSlug string                    `json:"specialist_slug"`
	Status         string                    `json:"status"`
	Limit          int                       `json:"limit"`
	Offset         int                       `json:"offset"`
	NextOffset     *int                      `json:"next_offset,omitempty"`
	Events         []SpecialistTimelineEvent `json:"events"`
	NextAction     string                    `json:"next_action"`
}

type SpecialistTimelineEvent struct {
	EventAt   time.Time `json:"event_at"`
	EventType string    `json:"event_type"`
	Source    string    `json:"source,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

type MessageResponse struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	NextAction string `json:"next_action"`
}
