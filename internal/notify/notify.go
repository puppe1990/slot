// Package notify is the transactional email boundary. The development
// implementation keeps a thread-safe outbox so handlers and tests can assert
// what a guest would have received without a mail server.
package notify

import (
	"context"
	"sync"
)

type Message struct {
	Kind    string
	To      string
	Subject string
	Body    string
}

type Notifier interface {
	Send(ctx context.Context, message Message) error
}

type Outbox struct {
	mu       sync.Mutex
	messages []Message
}

func NewOutbox() *Outbox {
	return &Outbox{}
}

func (o *Outbox) Send(_ context.Context, message Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.messages = append(o.messages, message)
	return nil
}

func (o *Outbox) Messages() []Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Message, len(o.messages))
	copy(out, o.messages)
	return out
}

func (o *Outbox) LastFor(address string) (Message, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.messages) - 1; i >= 0; i-- {
		if o.messages[i].To == address {
			return o.messages[i], true
		}
	}
	return Message{}, false
}
