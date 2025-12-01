package nostrutil

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
)

// ParsePubKey parses a nostr public key from hex, npub/nprofile, or nip05 identifier.
func ParsePubKey(value string) (nostr.PubKey, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nostr.PubKey{}, fmt.Errorf("empty pubkey")
	}

	if nip05.IsValidIdentifier(trimmed) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		profile, err := nip05.QueryIdentifier(ctx, trimmed)
		if err == nil {
			return profile.PublicKey, nil
		}
	}

	if pk, err := nostr.PubKeyFromHex(trimmed); err == nil {
		return pk, nil
	}

	if prefix, decoded, err := nip19.Decode(trimmed); err == nil {
		switch prefix {
		case "npub":
			if pk, ok := decoded.(nostr.PubKey); ok {
				return pk, nil
			}
		case "nprofile":
			if profile, ok := decoded.(nostr.ProfilePointer); ok {
				return profile.PublicKey, nil
			}
		}
	}

	return nostr.PubKey{}, fmt.Errorf("invalid pubkey %q", value)
}

// ParseEventID parses an event id from hex, note, or nevent strings.
func ParseEventID(value string) (nostr.ID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nostr.ID{}, fmt.Errorf("empty event id")
	}

	if id, err := nostr.IDFromHex(trimmed); err == nil {
		return id, nil
	}

	if prefix, decoded, err := nip19.Decode(trimmed); err == nil {
		switch prefix {
		case "note":
			if id, ok := decoded.(nostr.ID); ok {
				return id, nil
			}
		case "nevent":
			if ptr, ok := decoded.(nostr.EventPointer); ok {
				return ptr.ID, nil
			}
		}
	}

	return nostr.ID{}, fmt.Errorf("invalid event id %q", value)
}

// DecodeTagValue normalizes well-known nip19 tag values into their raw reference representation.
func DecodeTagValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "npub1") ||
		strings.HasPrefix(trimmed, "nevent1") ||
		strings.HasPrefix(trimmed, "note1") ||
		strings.HasPrefix(trimmed, "nprofile1") ||
		strings.HasPrefix(trimmed, "naddr1") {
		if ptr, err := nip19.ToPointer(trimmed); err == nil {
			return ptr.AsTagReference()
		}
	}
	return trimmed
}

// NiceRelayURL shortens a relay URL down to host[:port].
func NiceRelayURL(url string) string {
	normalized := nostr.NormalizeURL(url)
	parts := strings.SplitN(normalized, "/", 3)
	if len(parts) >= 3 {
		return parts[2]
	}
	if len(parts) >= 2 {
		return parts[1]
	}
	return normalized
}

// NiceRelayURLs maps urls through NiceRelayURL.
func NiceRelayURLs(urls []string) []string {
	nice := make([]string, len(urls))
	for i, url := range urls {
		nice[i] = NiceRelayURL(url)
	}
	return nice
}
