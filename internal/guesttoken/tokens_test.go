package guesttoken_test

import (
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/guesttoken"
)

const testSecret = "slot-test-secret-please-change"

func TestSignerRoundTripsBookingID(t *testing.T) {
	faker := gofakeit.New(42)
	signer, err := guesttoken.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		bookingID := int64(faker.IntRange(1, 1_000_000))
		token := signer.Sign(bookingID)
		got, err := signer.Parse(token)
		if err != nil {
			t.Fatalf("Parse(%q): %v", token, err)
		}
		if got != bookingID {
			t.Fatalf("Parse(%q) = %d, want %d", token, got, bookingID)
		}
	}
}

func TestSignerRejectsTamperedToken(t *testing.T) {
	signer, err := guesttoken.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	token := signer.Sign(77)

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("token %q, want two dot-separated parts", token)
	}
	tampered := parts[0] + "." + strings.Repeat("A", len(parts[1]))
	if _, err := signer.Parse(tampered); err == nil {
		t.Fatalf("Parse(%q) accepted a forged signature", tampered)
	}
}

func TestSignerRejectsOtherSecret(t *testing.T) {
	mine, err := guesttoken.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := guesttoken.New("a-completely-different-secret")
	if err != nil {
		t.Fatal(err)
	}
	token := mine.Sign(12)
	if _, err := theirs.Parse(token); err == nil {
		t.Fatal("a token signed with another secret was accepted")
	}
}

func TestSignerRejectsMalformedInput(t *testing.T) {
	signer, err := guesttoken.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "not-a-token", "abc.", ".abc", "a.b.c"} {
		if _, err := signer.Parse(token); err == nil {
			t.Fatalf("Parse(%q) accepted malformed input", token)
		}
	}
}

func TestNewRequiresLongSecret(t *testing.T) {
	if _, err := guesttoken.New("short"); err == nil {
		t.Fatal("New accepted a short secret")
	}
}

func TestHashIsStableAndHidesToken(t *testing.T) {
	signer, err := guesttoken.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	token := signer.Sign(9)
	first := guesttoken.Hash(token)
	second := guesttoken.Hash(token)
	if first != second {
		t.Fatal("Hash is not stable")
	}
	if guesttoken.Hash(token) == token {
		t.Fatal("Hash returned the raw token")
	}
	if len(guesttoken.Hash(token)) != 64 {
		t.Fatalf("Hash length = %d, want 64 hex chars", len(guesttoken.Hash(token)))
	}
}

func TestHoldTokenIsRandom(t *testing.T) {
	seen := make(map[string]struct{}, 50)
	for i := 0; i < 50; i++ {
		token := guesttoken.RandomToken()
		if _, dup := seen[token]; dup {
			t.Fatalf("RandomToken produced a duplicate: %q", token)
		}
		seen[token] = struct{}{}
	}
}
