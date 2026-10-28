package calendar

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const googleTokenURL = "https://oauth2.googleapis.com/token"

// serviceAccountScope is what the schedule needs: read busy time and write
// events. The account itself only sees the calendar shared with it, so the
// scope is not wider in practice than the calendar it can reach.
const serviceAccountScope = "https://www.googleapis.com/auth/calendar"

type serviceAccountKey struct {
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI     string `json:"token_uri"`
}

// ServiceAccountToken turns the downloaded credential JSON into an access-token
// source for the Google adapter. The token is cached until just before it
// expires, so a busy server does not mint one per request.
func ServiceAccountToken(rawJSON string) (func(context.Context) (string, error), error) {
	return serviceAccountToken(rawJSON, googleTokenURL, nil, time.Now)
}

func serviceAccountToken(rawJSON, tokenURL string, client *http.Client, now func() time.Time) (func(context.Context) (string, error), error) {
	var key serviceAccountKey
	if err := json.Unmarshal([]byte(rawJSON), &key); err != nil {
		return nil, fmt.Errorf("service account json: %w", err)
	}
	if strings.TrimSpace(key.ClientEmail) == "" || strings.TrimSpace(key.PrivateKey) == "" {
		return nil, fmt.Errorf("service account json: client_email and private_key are required")
	}
	privateKey, err := parseServiceAccountKey(key.PrivateKey)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	if tokenURL == "" {
		tokenURL = googleTokenURL
	}
	audience := key.TokenURI
	if audience == "" {
		audience = tokenURL
	}

	var mu sync.Mutex
	var token string
	var expiresAt time.Time

	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()

		if token != "" && now().Before(expiresAt.Add(-time.Minute)) {
			return token, nil
		}

		assertion, err := signAssertion(key.ClientEmail, key.PrivateKeyID, audience, privateKey, now())
		if err != nil {
			return "", err
		}

		form := url.Values{
			"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
			"assertion":  {assertion},
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return "", fmt.Errorf("token request: %w", err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		response, err := client.Do(request)
		if err != nil {
			return "", fmt.Errorf("token exchange: %w", err)
		}
		defer func() { _ = response.Body.Close() }()

		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		if response.StatusCode >= 300 {
			return "", fmt.Errorf("token exchange: %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", fmt.Errorf("token response: %w", err)
		}
		if payload.AccessToken == "" {
			return "", fmt.Errorf("token response: empty access_token")
		}
		ttl := payload.ExpiresIn
		if ttl <= 0 {
			ttl = 3600
		}
		token = payload.AccessToken
		expiresAt = now().Add(time.Duration(ttl) * time.Second)
		return token, nil
	}, nil
}

func signAssertion(email, keyID, audience string, key *rsa.PrivateKey, now time.Time) (string, error) {
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	if keyID != "" {
		header["kid"] = keyID
	}
	claims := map[string]any{
		"iss":   email,
		"scope": serviceAccountScope,
		"aud":   audience,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("sign assertion: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("sign assertion: %w", err)
	}

	signingInput := base64URL(headerJSON) + "." + base64URL(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign assertion: %w", err)
	}
	return signingInput + "." + base64URL(signature), nil
}

func base64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func parseServiceAccountKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("service account json: private_key is not PEM")
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("service account json: private_key is %T, want RSA", parsed)
		}
		return key, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("service account json: private_key: %w", err)
	}
	return key, nil
}
