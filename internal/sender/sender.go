package sender

import (
	"context"

	"notification-service/internal/models"
)

// Sender delivers a single notification through a specific channel
// (email, SMS, push). Implementations should return a descriptive error
// on failure so it can be recorded against the notification.
type Sender interface {
	Send(ctx context.Context, n *models.Notification) error
}
