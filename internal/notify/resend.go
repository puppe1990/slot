package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const resendBaseURL = "https://api.resend.com"

// Resend delivers through the REST API. The API key stays on the server and a
// failure returns the status without echoing the Authorization header.
type Resend struct {
	apiKey     string
	from       string
	baseURL    string
	httpClient *http.Client
}

func NewResend(apiKey, from, baseURL string) (*Resend, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("resend api key is required")
	}
	if strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("resend sender (EMAIL_FROM) is required")
	}
	if baseURL == "" {
		baseURL = resendBaseURL
	}
	return &Resend{
		apiKey:     apiKey,
		from:       from,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: http.DefaultClient,
	}, nil
}

func (r *Resend) Send(ctx context.Context, message Message) error {
	if strings.TrimSpace(message.To) == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"from":    r.from,
		"to":      []string{message.To},
		"subject": message.Subject,
		"text":    message.Body,
		"html":    message.HTML,
	})
	if err != nil {
		return fmt.Errorf("encode resend message: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+r.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := r.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call resend: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 256))
		return fmt.Errorf("resend returned %d: %s", response.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var sent struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&sent)
	// Id only: the recipient address is the guest's, no need to mirror it in
	// the server log. The kind is enough to trace a confirmation.
	log.Printf("notify: sent %s via resend (id %s)", message.Kind, sent.ID)
	return nil
}
