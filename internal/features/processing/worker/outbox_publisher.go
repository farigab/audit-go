package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"audit-go/internal/features/processing"
)

// OutboxPublisher moves durable outbox events into the local processing queue.
type OutboxPublisher struct {
	Log          zerolog.Logger
	repo         outboxPublisherRepository
	pollInterval time.Duration
}

type outboxPublisherRepository interface {
	PublishNextOutboxEvent(ctx context.Context) (*processing.OutboxEvent, error)
}

// NewOutboxPublisher creates a PostgreSQL-backed outbox publisher.
func NewOutboxPublisher(log zerolog.Logger, repo outboxPublisherRepository) *OutboxPublisher {
	return &OutboxPublisher{
		Log:          log,
		repo:         repo,
		pollInterval: pollInterval,
	}
}

// Start runs the publisher loop until ctx is cancelled.
func (p *OutboxPublisher) Start(ctx context.Context) {
	if !p.ready() {
		p.Log.Warn().Msg("outbox publisher disabled because repository is not configured")
		return
	}

	p.Log.Info().Msg("outbox publisher started")

	p.RunOnce(ctx)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.Log.Info().Msg("outbox publisher stopped")
			return
		case <-ticker.C:
			p.publish(ctx)
		}
	}
}

// RunOnce publishes at most one outbox event.
func (p *OutboxPublisher) RunOnce(ctx context.Context) bool {
	if !p.ready() {
		p.Log.Warn().Msg("outbox publisher skipped because repository is not configured")
		return false
	}

	return p.publish(ctx)
}

func (p *OutboxPublisher) publish(ctx context.Context) bool {
	event, err := p.repo.PublishNextOutboxEvent(ctx)
	if err != nil {
		log := p.Log.Error().Err(err)
		if event != nil {
			log = log.Str("outbox_event_id", event.ID).Str("event_type", event.EventType)
		}
		log.Msg("failed to publish outbox event")
		return event != nil
	}
	if event == nil {
		return false
	}

	p.Log.Info().
		Str("outbox_event_id", event.ID).
		Str("event_type", event.EventType).
		Msg("outbox event published")
	return true
}

func (p *OutboxPublisher) ready() bool {
	return p != nil && p.repo != nil
}
