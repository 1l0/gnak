package tabs

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/guigui-gui/guigui"
	"github.com/guigui-gui/guigui/basicwidget"

	"github.com/1l0/gnak/internal/state"
)

// Paste reproduces the inspector tab from vnak.
type Paste struct {
	guigui.DefaultWidget

	state *state.AppState

	inputLabel  basicwidget.Text
	inputText   basicwidget.TextInput
	outputLabel basicwidget.Text
	outputText  basicwidget.Text
}

// NewPaste creates a new paste tab widget.
func NewPaste(st *state.AppState) *Paste {
	return &Paste{state: st}
}

// Build wires child widgets and handlers.
func (p *Paste) Build(_ *guigui.Context, adder *guigui.ChildAdder) error {
	adder.AddChild(&p.inputLabel)
	adder.AddChild(&p.inputText)
	adder.AddChild(&p.outputLabel)
	adder.AddChild(&p.outputText)

	p.inputLabel.SetValue("Paste an event, filter, npub, nsec, nip05, nevent, or naddr:")
	p.inputLabel.SetBold(true)

	p.inputText.SetMultiline(true)
	p.inputText.SetAutoWrap(true)
	p.inputText.SetOnValueChanged(func(_ string, _ bool) {
		p.updateOutput()
	})

	p.outputLabel.SetValue("Decoded output")
	p.outputLabel.SetBold(true)
	p.outputText.SetAutoWrap(true)
	p.outputText.SetMultiline(true)
	p.outputText.SetSelectable(true)
	p.outputText.SetTabular(true)

	p.updateOutput()

	return nil
}

// Layout arranges the paste tab widgets.
func (p *Paste) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	layout := p.layoutSpec(context)
	layout.LayoutWidgets(context, widgetBounds.Bounds(), layouter)
}

// Measure reports the layout size so the scroll container can size itself.
func (p *Paste) Measure(context *guigui.Context, constraints guigui.Constraints) image.Point {
	layout := p.layoutSpec(context)
	return layout.Measure(context, constraints)
}

func (p *Paste) layoutSpec(context *guigui.Context) guigui.LinearLayout {
	u := basicwidget.UnitSize(context)
	return guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &p.inputLabel},
			{Widget: &p.inputText, Size: guigui.FlexibleSize(1)},
			{Widget: &p.outputLabel},
			{Widget: &p.outputText, Size: guigui.FlexibleSize(1)},
		},
	}
}

func (p *Paste) updateOutput() {
	text := strings.TrimSpace(p.inputText.Value())
	p.outputText.SetValue(describeInput(text))
}

func describeInput(text string) string {
	if text == "" {
		return "Waiting for input..."
	}

	if prefix, decoded, err := nip19.Decode(text); err == nil {
		return describeNIP19(prefix, decoded)
	}

	if nip05.IsValidIdentifier(text) {
		return fmt.Sprintf("%s looks like a NIP-05 identifier. Lookup support is not wired yet.", text)
	}

	if evt := tryDecodeEvent(text); evt != nil {
		return describeEvent(evt)
	}

	if filter := tryDecodeFilter(text); filter != nil {
		return describeFilter(filter)
	}

	if looksLikeHex(text) {
		return fmt.Sprintf("%s looks like hex, but it is not a valid secret or event id.", text)
	}

	return "Could not decode input."
}

func describeNIP19(prefix string, decoded any) string {
	switch prefix {
	case "nsec":
		if sk, ok := decoded.(nostr.SecretKey); ok {
			return describeSecretKey(sk)
		}
	case "npub":
		if pk, ok := decoded.(nostr.PubKey); ok {
			return describePubKey(pk)
		}
	case "note":
		if id, ok := decoded.(nostr.ID); ok {
			return fmt.Sprintf("note id: %s", id)
		}
	case "nevent":
		if pointer, ok := decoded.(nostr.EventPointer); ok {
			return describeEventPointer(pointer)
		}
	case "nprofile":
		if pointer, ok := decoded.(nostr.ProfilePointer); ok {
			return describeProfilePointer(pointer)
		}
	case "naddr":
		if pointer, ok := decoded.(nostr.EntityPointer); ok {
			return describeEntityPointer(pointer)
		}
	}

	data, _ := json.MarshalIndent(decoded, "", "  ")
	return fmt.Sprintf("decoded %s:\n%s", prefix, string(data))
}

func describeSecretKey(sk nostr.SecretKey) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "nsec: %s\n", nip19.EncodeNsec(sk))
	fmt.Fprintf(&sb, "hex: %s\n", sk.Hex())
	if npub, err := pubKeyFromSecret(sk); err == nil {
		fmt.Fprintf(&sb, "corresponding npub: %s", npub)
	}
	return sb.String()
}

func describePubKey(pk nostr.PubKey) string {
	return fmt.Sprintf("npub: %s\nhex: %s", nip19.EncodeNpub(pk), pk.Hex())
}

func describeEventPointer(pointer nostr.EventPointer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "nevent -> id: %s\n", pointer.ID.Hex())
	if pointer.Author != (nostr.PubKey{}) {
		fmt.Fprintf(&sb, "author: %s\n", pointer.Author.Hex())
	}
	if pointer.Kind != 0 {
		fmt.Fprintf(&sb, "kind: %d\n", pointer.Kind)
	}
	if len(pointer.Relays) > 0 {
		fmt.Fprintf(&sb, "relays: %s\n", strings.Join(pointer.Relays, ", "))
	}
	if pointer.Kind == 0 && pointer.Author == (nostr.PubKey{}) && len(pointer.Relays) == 0 {
		sb.WriteString("(no additional metadata provided)")
	}
	return sb.String()
}

func describeProfilePointer(pointer nostr.ProfilePointer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "nprofile -> pubkey: %s\n", pointer.PublicKey.Hex())
	if len(pointer.Relays) > 0 {
		fmt.Fprintf(&sb, "relays: %s", strings.Join(pointer.Relays, ", "))
	}
	return sb.String()
}

func describeEntityPointer(pointer nostr.EntityPointer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "naddr -> kind: %d\n", pointer.Kind)
	fmt.Fprintf(&sb, "pubkey: %s\n", pointer.PublicKey.Hex())
	fmt.Fprintf(&sb, "identifier: %s\n", pointer.Identifier)
	if len(pointer.Relays) > 0 {
		fmt.Fprintf(&sb, "relays: %s", strings.Join(pointer.Relays, ", "))
	}
	return sb.String()
}

func describeEvent(evt *nostr.Event) string {
	payload, _ := json.MarshalIndent(evt, "", "  ")
	timestamp := time.Unix(int64(evt.CreatedAt), 0).UTC().Format(time.RFC3339)
	var sb strings.Builder
	fmt.Fprintf(&sb, "event %s\n", evt.ID.Hex())
	fmt.Fprintf(&sb, "kind: %d\n", evt.Kind)
	fmt.Fprintf(&sb, "created at: %s\n", timestamp)
	fmt.Fprintf(&sb, "pubkey: %s\n", evt.PubKey.Hex())
	fmt.Fprintf(&sb, "tags: %d\n", len(evt.Tags))
	if evt.Content != "" {
		fmt.Fprintf(&sb, "content preview: %s\n", truncate(evt.Content, 200))
	}
	sb.WriteString("\nJSON:\n")
	sb.Write(payload)
	return sb.String()
}

func describeFilter(filter *nostr.Filter) string {
	payload, _ := json.MarshalIndent(filter, "", "  ")
	return "filter JSON:\n" + string(payload)
}

func pubKeyFromSecret(sk nostr.SecretKey) (string, error) {
	_, pub := btcec.PrivKeyFromBytes(sk[:])
	pk := nostr.PubKey(pub.SerializeCompressed()[1:])
	return nip19.EncodeNpub(pk), nil
}

func tryDecodeEvent(text string) *nostr.Event {
	var evt nostr.Event
	if err := json.Unmarshal([]byte(text), &evt); err != nil {
		return nil
	}
	if evt.ID == (nostr.ID{}) && evt.Kind == 0 && evt.CreatedAt == 0 && evt.Content == "" && len(evt.Tags) == 0 {
		return nil
	}
	if evt.PubKey == (nostr.PubKey{}) && evt.Sig == ([64]byte{}) {
		return nil
	}
	return &evt
}

func tryDecodeFilter(text string) *nostr.Filter {
	var filter nostr.Filter
	if err := json.Unmarshal([]byte(text), &filter); err != nil {
		return nil
	}
	if isZeroFilter(filter) {
		return nil
	}
	return &filter
}

func isZeroFilter(filter nostr.Filter) bool {
	return len(filter.Kinds) == 0 && len(filter.Authors) == 0 && len(filter.IDs) == 0 && len(filter.Tags) == 0 && filter.Since == 0 && filter.Until == 0 && filter.Limit == 0 && filter.Search == "" && !filter.LimitZero
}

func looksLikeHex(text string) bool {
	if len(text)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(text)
	return err == nil
}

func truncate(content string, max int) string {
	if len(content) <= max {
		return content
	}
	return content[:max] + "..."
}
