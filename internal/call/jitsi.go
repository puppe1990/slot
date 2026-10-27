package call

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const jitsiBaseURL = "https://meet.jit.si"

// Jitsi needs no API: the room name is the URL. The suffix is random so a
// schedule page cannot be crawled into someone else's call.
type Jitsi struct {
	baseURL string
}

func NewJitsi(baseURL string) (*Jitsi, error) {
	if baseURL == "" {
		baseURL = jitsiBaseURL
	}
	return &Jitsi{baseURL: strings.TrimRight(baseURL, "/")}, nil
}

func (j *Jitsi) Provider() string { return ProviderJitsi }

func (j *Jitsi) CreateRoom(_ context.Context, input Input) (Room, error) {
	return Room{
		URL:      j.baseURL + "/slot-" + randomHex(12),
		Provider: ProviderJitsi,
	}, nil
}

func (j *Jitsi) RoomURL(_ context.Context, room Room, _ Viewer) (string, error) {
	return room.URL, nil
}

// EndRoom is a no-op: a Jitsi room exists only while someone is in it.
func (j *Jitsi) EndRoom(context.Context, Room) error { return nil }

func roomName(bookingID int64) string {
	return fmt.Sprintf("slot-%d-%s", bookingID, randomHex(4))
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("call: crypto/rand: %v", err))
	}
	return hex.EncodeToString(buffer)
}
