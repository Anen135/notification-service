package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"notification-service/internal/models"
)

// PushSender delivers push notifications via Firebase Cloud Messaging's
// legacy HTTP API. n.Recipient is expected to hold the target device/registration token.
type PushSender struct {
	ServerKey  string
	Endpoint   string
	HTTPClient *http.Client
}

func NewPushSender(serverKey, endpoint string) *PushSender {
	return &PushSender{ServerKey: serverKey, Endpoint: endpoint, HTTPClient: &http.Client{}}
}

type fcmPayload struct {
	To           string            `json:"to"`
	Notification fcmNotifBody      `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
}

type fcmNotifBody struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
}

func (s *PushSender) Send(ctx context.Context, n *models.Notification) error {
	if s.ServerKey == "" {
		return fmt.Errorf("fcm server key not configured")
	}

	payload := fcmPayload{
		To: n.Recipient,
		Notification: fcmNotifBody{
			Title: n.Subject,
			Body:  n.Body,
		},
		Data: n.Metadata,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal fcm payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build fcm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+s.ServerKey)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fcm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("fcm returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
