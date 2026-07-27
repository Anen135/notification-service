package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"notification-service/internal/models"
	"notification-service/internal/queue"
	"notification-service/internal/repository"
	"notification-service/internal/sender"
)

type Service struct {
	repo  *repository.Repository
	queue *queue.RabbitMQ

	senders map[models.Channel]sender.Sender

	maxAttempts  int
	retryDelayMS int
}

func New(repo *repository.Repository, q *queue.RabbitMQ, senders map[models.Channel]sender.Sender, maxAttempts, retryDelayMS int) *Service {
	return &Service{
		repo:         repo,
		queue:        q,
		senders:      senders,
		maxAttempts:  maxAttempts,
		retryDelayMS: retryDelayMS,
	}
}

func (s *Service) CreateAndEnqueue(ctx context.Context, req models.CreateNotificationRequest) (*models.Notification, error) {
	if req.Channel != models.ChannelEmail && req.Channel != models.ChannelSMS && req.Channel != models.ChannelPush {
		return nil, fmt.Errorf("invalid channel %q", req.Channel)
	}
	if req.Recipient == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	subject, body := req.Subject, req.Body

	if req.TemplateName != "" {
		tpl, err := s.repo.GetTemplateByName(ctx, req.TemplateName)
		if err != nil {
			return nil, fmt.Errorf("template %q not found: %w", req.TemplateName, err)
		}
		subject = render(tpl.Subject, req.TemplateData)
		body = render(tpl.Body, req.TemplateData)
	}

	if body == "" {
		return nil, fmt.Errorf("body or template_name with resolvable content is required")
	}

	now := time.Now().UTC()
	n := &models.Notification{
		ID:          uuid.New(),
		Channel:     req.Channel,
		Recipient:   req.Recipient,
		Subject:     subject,
		Body:        body,
		Status:      models.StatusPending,
		Attempts:    0,
		MaxAttempts: s.maxAttempts,
		Metadata:    req.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateNotification(ctx, n); err != nil {
		return nil, fmt.Errorf("save notification: %w", err)
	}

	if err := s.queue.Publish(ctx, n.Channel, models.QueueMessage{NotificationID: n.ID}); err != nil {
		return nil, fmt.Errorf("publish notification: %w", err)
	}

	return n, nil
}

func (s *Service) GetNotification(ctx context.Context, id uuid.UUID) (*models.Notification, error) {
	return s.repo.GetNotification(ctx, id)
}

func (s *Service) ListNotifications(ctx context.Context, status models.Status, limit, offset int) ([]*models.Notification, error) {
	return s.repo.ListNotifications(ctx, status, limit, offset)
}

func (s *Service) CreateTemplate(ctx context.Context, t *models.Template) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now().UTC()
	return s.repo.CreateTemplate(ctx, t)
}

func (s *Service) ProcessMessage(ctx context.Context, msg models.QueueMessage) error {
	n, err := s.repo.GetNotification(ctx, msg.NotificationID)
	if err != nil {
		return fmt.Errorf("load notification %s: %w", msg.NotificationID, err)
	}

	// Already delivered (e.g. duplicate delivery after a redelivery/ack race) - nothing to do.
	if n.Status == models.StatusSent {
		return nil
	}

	snd, ok := s.senders[n.Channel]
	if !ok {
		return fmt.Errorf("no sender registered for channel %q", n.Channel)
	}

	attempts := n.Attempts + 1
	sendErr := snd.Send(ctx, n)

	if sendErr == nil {
		return s.repo.UpdateAfterAttempt(ctx, n.ID, models.StatusSent, attempts, "")
	}

	log.Printf("delivery failed for notification %s (channel=%s, attempt=%d/%d): %v",
		n.ID, n.Channel, attempts, n.MaxAttempts, sendErr)

	if attempts >= n.MaxAttempts {
		if err := s.repo.UpdateAfterAttempt(ctx, n.ID, models.StatusFailed, attempts, sendErr.Error()); err != nil {
			return err
		}
		return s.queue.PublishFailed(ctx, msg)
	}

	if err := s.repo.UpdateAfterAttempt(ctx, n.ID, models.StatusRetrying, attempts, sendErr.Error()); err != nil {
		return err
	}
	return s.queue.PublishRetry(ctx, n.Channel, msg, s.retryDelayMS)
}

func render(tpl string, data map[string]string) string {
	out := tpl
	for k, v := range data {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}
