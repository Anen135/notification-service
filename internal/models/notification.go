package models

import (
	"time"

	"github.com/google/uuid"
)

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
	ChannelPush  Channel = "push"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusSent     Status = "sent"
	StatusFailed   Status = "failed"
	StatusRetrying Status = "retrying"
)

// Notification is the persisted representation of a single notification request.
type Notification struct {
	ID          uuid.UUID         `json:"id"`
	Channel     Channel           `json:"channel"`
	Recipient   string            `json:"recipient"`
	Subject     string            `json:"subject,omitempty"`
	Body        string            `json:"body"`
	Status      Status            `json:"status"`
	Attempts    int               `json:"attempts"`
	MaxAttempts int               `json:"max_attempts"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Error       string            `json:"error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	SentAt      *time.Time        `json:"sent_at,omitempty"`
}

// CreateNotificationRequest is the payload accepted by POST /api/v1/notifications.
type CreateNotificationRequest struct {
	Channel      Channel           `json:"channel"`
	Recipient    string            `json:"recipient"`
	Subject      string            `json:"subject,omitempty"`
	Body         string            `json:"body,omitempty"`
	TemplateName string            `json:"template_name,omitempty"`
	TemplateData map[string]string `json:"template_data,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Template is a reusable message template for a given channel.
type Template struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Channel   Channel   `json:"channel"`
	Subject   string    `json:"subject,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// QueueMessage is the payload published to RabbitMQ; only the notification ID
// is required since the worker re-reads full state from PostgreSQL (avoids
// stale data if the notification was edited between publish and consume).
type QueueMessage struct {
	NotificationID uuid.UUID `json:"notification_id"`
}
