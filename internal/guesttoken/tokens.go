package guesttoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const minSecretLength = 16

// Signer protects the guest-facing links. The database stores only Hash(token),
// so a leaked backup or SQL log cannot be replayed as a booking link.
type Signer struct {
	secret []byte
}

func New(secret string) (*Signer, error) {
	if len(secret) < minSecretLength {
		return nil, fmt.Errorf("guest token secret has %d bytes, want at least %d", len(secret), minSecretLength)
	}
	return &Signer{secret: []byte(secret)}, nil
}

func (s *Signer) Sign(bookingID int64) string {
	payload := strconv.FormatInt(bookingID, 10)
	return payload + "." + s.signature(payload)
}

func (s *Signer) Parse(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, fmt.Errorf("guest token %q, want <id>.<signature>", token)
	}
	expected := s.signature(parts[0])
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return 0, errors.New("guest token signature does not match")
	}
	bookingID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("guest token id %q is not a number", parts[0])
	}
	return bookingID, nil
}

func (s *Signer) signature(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RandomToken is the short-lived hold bearer. 32 random bytes keep it
// unguessable even if a token shows up in a referrer log.
func RandomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand never fails on a supported platform; a silent all-zero
		// token would be worse than panicking on the request.
		panic(fmt.Sprintf("guesttoken: crypto/rand: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
