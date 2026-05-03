package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"audit-go/internal/features/processing"
)

func TestOutboxPublisherRunOncePublishesEvent(t *testing.T) {
	repo := &fakeOutboxPublisherRepository{
		event: &processing.OutboxEvent{
			ID:        "00000000-0000-0000-0000-000000000040",
			EventType: processing.EventDocumentUploaded,
		},
	}
	publisher := NewOutboxPublisher(zerolog.Nop(), repo)

	if published := publisher.RunOnce(context.Background()); !published {
		t.Fatal("expected one outbox event to be published")
	}
	if repo.calls != 1 {
		t.Fatalf("expected one publish call, got %d", repo.calls)
	}
}

func TestOutboxPublisherRunOnceReturnsFalseWhenNoEvent(t *testing.T) {
	repo := &fakeOutboxPublisherRepository{}
	publisher := NewOutboxPublisher(zerolog.Nop(), repo)

	if published := publisher.RunOnce(context.Background()); published {
		t.Fatal("did not expect an outbox event to be published")
	}
}

func TestOutboxPublisherRunOnceReportsAttemptedFailure(t *testing.T) {
	repo := &fakeOutboxPublisherRepository{
		event: &processing.OutboxEvent{
			ID:        "00000000-0000-0000-0000-000000000040",
			EventType: processing.EventDocumentUploaded,
		},
		err: errors.New("publish failed"),
	}
	publisher := NewOutboxPublisher(zerolog.Nop(), repo)

	if published := publisher.RunOnce(context.Background()); !published {
		t.Fatal("expected attempted publish to be reported")
	}
}

type fakeOutboxPublisherRepository struct {
	event *processing.OutboxEvent
	err   error
	calls int
}

func (f *fakeOutboxPublisherRepository) PublishNextOutboxEvent(context.Context) (*processing.OutboxEvent, error) {
	f.calls++
	return f.event, f.err
}
