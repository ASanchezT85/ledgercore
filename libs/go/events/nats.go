package events

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
)

// StreamName is the single JetStream stream that stores all platform events.
const StreamName = "LEDGERCORE"

// streamSubjects covers every topic in the platform.
var streamSubjects = []string{"ledger.>", "recon.>", "identity.>"}

// NATSPublisher publishes envelopes to NATS JetStream. Create it with
// NewNATSPublisher, which also ensures the LEDGERCORE stream exists.
type NATSPublisher struct {
	js nats.JetStreamContext
}

// NewNATSPublisher wraps an existing NATS connection with a JetStream
// context and creates the LEDGERCORE stream if it does not exist yet.
func NewNATSPublisher(nc *nats.Conn) (*NATSPublisher, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("events: jetstream context: %w", err)
	}
	info, err := js.StreamInfo(StreamName)
	switch {
	case errors.Is(err, nats.ErrStreamNotFound):
		_, err = js.AddStream(&nats.StreamConfig{
			Name:      StreamName,
			Subjects:  streamSubjects,
			Storage:   nats.FileStorage,
			Retention: nats.LimitsPolicy,
		})
	case err == nil && !sameSubjects(info.Config.Subjects, streamSubjects):
		// The stream predates a new subject family (e.g. identity.>):
		// widen its subject list in place, keeping the rest of the config.
		cfg := info.Config
		cfg.Subjects = streamSubjects
		_, err = js.UpdateStream(&cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("events: ensure stream %s: %w", StreamName, err)
	}
	return &NATSPublisher{js: js}, nil
}

// sameSubjects reports whether both subject lists contain the same set.
func sameSubjects(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}

// Publish sends the envelope to its topic. The envelope id doubles as the
// JetStream message id, so redeliveries from the outbox poller deduplicate
// on the server side.
func (p *NATSPublisher) Publish(env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("events: marshal envelope %s: %w", env.ID, err)
	}
	_, err = p.js.Publish(env.Type, body, nats.MsgId(env.ID.String()))
	if err != nil {
		return fmt.Errorf("events: publish %s to %s: %w", env.ID, env.Type, err)
	}
	return nil
}
