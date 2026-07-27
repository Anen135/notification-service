package sender

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"notification-service/internal/models"
)

// SMSSender posts to a Twilio-compatible REST endpoint:
//
//	POST {ProviderURL}
//	Authorization: Basic base64(SID:Token)
//	Body: application/x-www-form-urlencoded  From=...&To=...&Body=...
//
// Swap ProviderURL/credentials in config to point at any provider that
// accepts the same form-encoded contract (Twilio, Vonage-compatible proxies, etc).
type SMSSender struct {
	ProviderURL string
	SID         string
	Token       string
	From        string
	HTTPClient  *http.Client
}

func NewSMSSender(providerURL, sid, token, from string) *SMSSender {
	return &SMSSender{
		ProviderURL: providerURL,
		SID:         sid,
		Token:       token,
		From:        from,
		HTTPClient:  &http.Client{},
	}
}

func (s *SMSSender) Send(ctx context.Context, n *models.Notification) error {
	if s.ProviderURL == "" {
		return fmt.Errorf("sms provider url not configured")
	}

	form := url.Values{}
	form.Set("From", s.From)
	form.Set("To", n.Recipient)
	form.Set("Body", n.Body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ProviderURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build sms request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.SID, s.Token)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("sms request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("sms provider returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
