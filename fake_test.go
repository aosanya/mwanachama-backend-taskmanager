package mwanachamataskmanager_test

import "context"

// publishedEvent is one recorded call to [recordingPublisher.Publish].
type publishedEvent struct {
	Topic   string
	Payload any
}

// recordingPublisher is the test-side implementation of [events.Publisher].
// It records every call for assertions and also derives a topic-only
// projection (events) for the legacy assertion form.
type recordingPublisher struct {
	full   []publishedEvent
	events []string // topic-only projection of full
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, payload any) error {
	p.full = append(p.full, publishedEvent{Topic: topic, Payload: payload})
	p.events = append(p.events, topic)
	return nil
}

// findEvent returns the first recorded event matching topic.
func findEvent(events []publishedEvent, topic string) (publishedEvent, bool) {
	for _, e := range events {
		if e.Topic == topic {
			return e, true
		}
	}
	return publishedEvent{}, false
}
