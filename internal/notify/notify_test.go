package notify_test

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/slot/internal/notify"
)

func TestOutboxKeepsOrder(t *testing.T) {
	ctx := context.Background()
	faker := gofakeit.New(31)
	outbox := notify.NewOutbox()

	for i := 0; i < 3; i++ {
		message := notify.Message{
			Kind:    "booking.confirmed",
			To:      faker.Email(),
			Subject: faker.Sentence(3),
			Body:    faker.Sentence(10),
		}
		if err := outbox.Send(ctx, message); err != nil {
			t.Fatal(err)
		}
	}

	messages := outbox.Messages()
	if len(messages) != 3 {
		t.Fatalf("Messages() = %d, want 3", len(messages))
	}
	for i := 1; i < len(messages); i++ {
		if messages[i-1].Kind != messages[i].Kind {
			t.Fatalf("message %d changed kind: %+v", i, messages)
		}
	}
}

func TestOutboxLastFor(t *testing.T) {
	ctx := context.Background()
	outbox := notify.NewOutbox()

	if err := outbox.Send(ctx, notify.Message{To: "guest@example.com", Subject: "Confirmed"}); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Send(ctx, notify.Message{To: "other@example.com", Subject: "Ignored"}); err != nil {
		t.Fatal(err)
	}

	last, ok := outbox.LastFor("guest@example.com")
	if !ok || last.Subject != "Confirmed" {
		t.Fatalf("LastFor = %+v, %v", last, ok)
	}
	if _, ok := outbox.LastFor("missing@example.com"); ok {
		t.Fatal("LastFor found a message for an address that never received one")
	}
}

func TestOutboxMessagesAreCopies(t *testing.T) {
	ctx := context.Background()
	outbox := notify.NewOutbox()
	if err := outbox.Send(ctx, notify.Message{To: "guest@example.com", Subject: "One"}); err != nil {
		t.Fatal(err)
	}

	messages := outbox.Messages()
	messages[0].Subject = "mutated"
	if outbox.Messages()[0].Subject != "One" {
		t.Fatal("Messages() exposed the internal slice")
	}
}
