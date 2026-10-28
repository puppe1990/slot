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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testServiceAccount(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	raw, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"client_email":   "slot@project.iam.gserviceaccount.com",
		"private_key":    pemKey,
		"private_key_id": "key-id",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), key
}

func decodeSegment(t *testing.T, segment string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("segment %q: %v", segment, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("segment %q: %v", segment, err)
	}
	return payload
}

func TestServiceAccountTokenExchangesSignedJWT(t *testing.T) {
	rawJSON, key := testServiceAccount(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("grant_type = %q", got)
		}
		assertion := r.Form.Get("assertion")
		parts := strings.Split(assertion, ".")
		if len(parts) != 3 {
			t.Fatalf("assertion has %d parts", len(parts))
		}
		header := decodeSegment(t, parts[0])
		claims := decodeSegment(t, parts[1])
		if header["alg"] != "RS256" || header["kid"] != "key-id" {
			t.Errorf("header = %v", header)
		}
		if claims["iss"] != "slot@project.iam.gserviceaccount.com" {
			t.Errorf("iss = %v", claims["iss"])
		}
		if claims["scope"] != serviceAccountScope {
			t.Errorf("scope = %v", claims["scope"])
		}
		if claims["aud"] != "https://oauth2.googleapis.com/token" {
			t.Errorf("aud = %v", claims["aud"])
		}
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
			t.Errorf("signature does not verify: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ya29.test","expires_in":3600}`))
	}))
	defer server.Close()

	fixed := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	source, err := serviceAccountToken(rawJSON, server.URL, server.Client(), func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}

	token, err := source(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "ya29.test" {
		t.Fatalf("token = %q", token)
	}
	if _, err := source(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("token endpoint called %d times, want 1 (cache)", got)
	}
}

func TestServiceAccountTokenRefreshesBeforeExpiry(t *testing.T) {
	rawJSON, _ := testServiceAccount(t)
	var calls int32
	var current atomic.Value
	current.Store(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"access_token":"token-` + string(rune('0'+n)) + `","expires_in":3600}`))
	}))
	defer server.Close()

	source, err := serviceAccountToken(rawJSON, server.URL, server.Client(), func() time.Time {
		return current.Load().(time.Time)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source(context.Background()); err != nil {
		t.Fatal(err)
	}

	current.Store(time.Date(2026, 10, 28, 13, 5, 0, 0, time.UTC))
	second, err := source(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second != "token-2" || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("token = %q, calls = %d, want a refresh", second, calls)
	}
}

func TestServiceAccountRejectsBrokenCredentials(t *testing.T) {
	if _, err := ServiceAccountToken("not json"); err == nil {
		t.Fatal("invalid json should fail")
	}
	if _, err := ServiceAccountToken(`{"client_email":"x@y","private_key":"nope"}`); err == nil {
		t.Fatal("non-PEM key should fail")
	}
	if _, err := ServiceAccountToken(`{"client_email":"x@y"}`); err == nil {
		t.Fatal("missing private key should fail")
	}
}

func TestNewPrefersServiceAccount(t *testing.T) {
	rawJSON, _ := testServiceAccount(t)
	provider, err := New(Config{GoogleServiceAccount: rawJSON, CalendarID: "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Provider() != ProviderGoogle {
		t.Fatalf("provider = %s, want google", provider.Provider())
	}
	google, ok := provider.(*Google)
	if !ok || google.CalendarID != "owner@example.com" || google.AccessToken == nil {
		t.Fatalf("provider = %#v", provider)
	}
}
