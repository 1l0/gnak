package event

import (
	"context"
	"errors"
	"fmt"

	"fiatjaf.com/nostr"
)

// ErrNoRelays indicates that publish was requested without any relay URLs.
var ErrNoRelays = errors.New("no relay URLs provided")

// Publisher wraps a nostr pool to publish events to multiple relays.
type Publisher struct {
	pool *nostr.Pool
}

// Outcome captures the result reported by a relay publish attempt.
type Outcome struct {
	Relay string
	Err   error
}

// NewPublisher initializes a Publisher with sane defaults.
func NewPublisher(pool *nostr.Pool) *Publisher {
	if pool == nil {
		pool = nostr.NewPool(nostr.PoolOptions{PenaltyBox: true})
	}
	return &Publisher{pool: pool}
}

// Publish pushes the given event to every relay, returning one outcome per relay.
// The caller is responsible for providing a context with the desired timeout.
func (p *Publisher) Publish(ctx context.Context, relays []string, evt nostr.Event) ([]Outcome, error) {
	normalized := normalizeRelays(relays)
	if len(normalized) == 0 {
		return nil, ErrNoRelays
	}

	resultCh := p.pool.PublishMany(ctx, normalized, evt)
	outcomes := make([]Outcome, 0, len(normalized))
	for res := range resultCh {
		relayURL := res.RelayURL
		if relayURL == "" && res.Relay != nil {
			relayURL = res.Relay.URL
		}
		if relayURL == "" {
			relayURL = fmt.Sprintf("relay-%d", len(outcomes)+1)
		}
		outcomes = append(outcomes, Outcome{Relay: relayURL, Err: res.Error})
	}
	return outcomes, nil
}

func normalizeRelays(relays []string) []string {
	if len(relays) == 0 {
		return nil
	}
	dedup := make(map[string]struct{}, len(relays))
	ordered := make([]string, 0, len(relays))
	for _, raw := range relays {
		norm := nostr.NormalizeURL(raw)
		if norm == "" {
			continue
		}
		if _, exists := dedup[norm]; exists {
			continue
		}
		dedup[norm] = struct{}{}
		ordered = append(ordered, norm)
	}
	return ordered
}
