package sender

import (
	"context"
	"fmt"
	"net/smtp"

	"notification-service/internal/models"
)

type EmailSender struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

func NewEmailSender(host, port, user, pass, from string) *EmailSender {
	return &EmailSender{Host: host, Port: port, User: user, Pass: pass, From: from}
}

func (s *EmailSender) Send(ctx context.Context, n *models.Notification) error {
	addr := fmt.Sprintf("%s:%s", s.Host, s.Port)

	msg := fmt.Appendf(nil,
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s\r\n",
		s.From, n.Recipient, n.Subject, n.Body,
	)

	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Pass, s.Host)
	}

	// net/smtp has no context support; the queue-level retry/backoff handles
	// timeouts and transient failures at a higher level.
	if err := smtp.SendMail(addr, auth, s.From, []string{n.Recipient}, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
