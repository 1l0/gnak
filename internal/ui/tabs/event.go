package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/atotto/clipboard"
	"github.com/guigui-gui/guigui"
	"github.com/guigui-gui/guigui/basicwidget"

	"fiatjaf.com/nostr"

	"github.com/1l0/gnak/internal/event"
	"github.com/1l0/gnak/internal/state"
)

// Event renders the event composer UI.
type Event struct {
	guigui.DefaultWidget

	state     *state.AppState
	publisher *event.Publisher

	kindLabel      basicwidget.Text
	kindInput      basicwidget.NumberInput
	createdAtLabel basicwidget.Text
	createdAtInput basicwidget.TextInput
	createdAtNow   basicwidget.Button

	contentLabel basicwidget.Text
	contentInput basicwidget.TextInput

	tagsLabel basicwidget.Text
	tagsHelp  basicwidget.Text
	tagsInput basicwidget.TextInput

	relaysLabel basicwidget.Text
	relaysHelp  basicwidget.Text
	relaysInput basicwidget.TextInput

	copyButton    basicwidget.Button
	publishButton basicwidget.Button

	previewLabel basicwidget.Text
	previewText  basicwidget.Text

	publishing atomic.Bool
}

// NewEvent creates an event tab bound to shared state.
func NewEvent(st *state.AppState, publisher *event.Publisher) *Event {
	return &Event{state: st, publisher: publisher}
}

// Build wires widgets and handlers.
func (e *Event) Build(_ *guigui.Context, adder *guigui.ChildAdder) error {
	for _, widget := range []guigui.Widget{
		&e.kindLabel,
		&e.kindInput,
		&e.createdAtLabel,
		&e.createdAtInput,
		&e.createdAtNow,
		&e.contentLabel,
		&e.contentInput,
		&e.tagsLabel,
		&e.tagsHelp,
		&e.tagsInput,
		&e.relaysLabel,
		&e.relaysHelp,
		&e.relaysInput,
		&e.copyButton,
		&e.publishButton,
		&e.previewLabel,
		&e.previewText,
	} {
		adder.AddChild(widget)
	}

	e.kindLabel.SetValue("Kind")
	e.kindLabel.SetBold(true)
	e.kindInput.SetMinimumValue(0)
	e.kindInput.SetMaximumValue(int(^uint16(0)))
	e.kindInput.SetValue(1)
	e.kindInput.SetOnValueChanged(func(int, bool) {
		e.updatePreview()
	})

	e.createdAtLabel.SetValue("Created at (unix seconds)")
	e.createdAtLabel.SetBold(true)
	e.createdAtNow.SetText("Now")
	e.createdAtNow.SetOnUp(func() {
		e.setCurrentTimestamp()
		e.updatePreview()
	})
	e.createdAtInput.SetOnValueChanged(func(string, bool) {
		e.updatePreview()
	})

	e.contentLabel.SetValue("Content")
	e.contentLabel.SetBold(true)
	e.contentInput.SetMultiline(true)
	e.contentInput.SetAutoWrap(true)
	e.contentInput.SetOnValueChanged(func(string, bool) {
		e.updatePreview()
	})

	e.tagsLabel.SetValue("Tags")
	e.tagsLabel.SetBold(true)
	e.tagsHelp.SetValue("Enter one tag per line. Format: kind value1 value2 ... (example: e abc123 https://relay)")
	e.tagsHelp.SetAutoWrap(true)
	e.tagsInput.SetMultiline(true)
	e.tagsInput.SetAutoWrap(true)
	e.tagsInput.SetOnValueChanged(func(string, bool) {
		e.updatePreview()
	})

	e.relaysLabel.SetValue("Relays")
	e.relaysLabel.SetBold(true)
	e.relaysHelp.SetValue("One relay URL per line. These relays are used when publishing.")
	e.relaysHelp.SetAutoWrap(true)
	e.relaysInput.SetMultiline(true)
	e.relaysInput.SetAutoWrap(true)
	e.relaysInput.SetOnValueChanged(func(string, bool) {
		e.updatePreview()
	})

	e.copyButton.SetText("Copy JSON")
	e.copyButton.SetOnUp(func() {
		if text := e.previewText.Value(); strings.TrimSpace(text) != "" {
			if err := clipboard.WriteAll(text); err != nil {
				e.state.SetStatus("failed to copy event JSON: " + err.Error())
				return
			}
			e.state.SetStatus("event JSON copied to clipboard")
		}
	})

	e.publishButton.SetText("Publish")
	e.publishButton.SetOnUp(e.handlePublish)

	e.previewLabel.SetValue("Preview (unsigned event JSON)")
	e.previewLabel.SetBold(true)
	e.previewText.SetSelectable(true)
	e.previewText.SetMultiline(true)
	e.previewText.SetAutoWrap(true)
	e.previewText.SetTabular(true)

	e.setCurrentTimestamp()
	e.updatePreview()
	return nil
}

// Layout arranges the composer controls.
func (e *Event) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	layout := e.layoutSpec(context)
	layout.LayoutWidgets(context, widgetBounds.Bounds(), layouter)
}

// Measure reports the size of the composed layout (needed for scroll containers).
func (e *Event) Measure(context *guigui.Context, constraints guigui.Constraints) image.Point {
	layout := e.layoutSpec(context)
	return layout.Measure(context, constraints)
}

func (e *Event) layoutSpec(context *guigui.Context) guigui.LinearLayout {
	u := basicwidget.UnitSize(context)
	createdAtRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 4,
		Items: []guigui.LinearLayoutItem{
			{Widget: &e.createdAtInput, Size: guigui.FlexibleSize(1)},
			{Widget: &e.createdAtNow, Size: guigui.FixedSize(4 * u)},
		},
	}
	headerRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &e.kindLabel, Size: guigui.FixedSize(4 * u)},
			{Widget: &e.kindInput, Size: guigui.FixedSize(6 * u)},
			{Widget: &e.createdAtLabel, Size: guigui.FixedSize(6 * u)},
			{Layout: createdAtRow, Size: guigui.FlexibleSize(1)},
		},
	}
	buttonsRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &e.copyButton, Size: guigui.FixedSize(6 * u)},
			{Widget: &e.publishButton, Size: guigui.FixedSize(6 * u)},
		},
	}
	return guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 2,
		Padding:   guigui.Padding{Start: u, End: u, Top: u, Bottom: u},
		Items: []guigui.LinearLayoutItem{
			{Layout: headerRow},
			{Widget: &e.contentLabel},
			{Widget: &e.contentInput, Size: guigui.FlexibleSize(2)},
			{Widget: &e.tagsLabel},
			{Widget: &e.tagsHelp},
			{Widget: &e.tagsInput, Size: guigui.FlexibleSize(1)},
			{Widget: &e.relaysLabel},
			{Widget: &e.relaysHelp},
			{Widget: &e.relaysInput, Size: guigui.FlexibleSize(1)},
			{Layout: buttonsRow},
			{Widget: &e.previewLabel},
			{Widget: &e.previewText, Size: guigui.FlexibleSize(2)},
		},
	}
}

// Tick keeps button state in sync with the available secret key.
func (e *Event) Tick(context *guigui.Context, _ *guigui.WidgetBounds) error {
	_, hasSecret := e.state.SecretKey()
	context.SetEnabled(&e.publishButton, hasSecret && !e.publishing.Load())
	return nil
}

func (e *Event) handlePublish() {
	if e.publisher == nil {
		e.state.SetStatus("event publisher not initialized")
		return
	}
	relays := e.Relays()
	if len(relays) == 0 {
		e.state.SetStatus("add at least one relay URL before publishing")
		return
	}
	sk, ok := e.state.SecretKey()
	if !ok {
		e.state.SetStatus("enter a secret key before publishing")
		return
	}
	if !e.publishing.CompareAndSwap(false, true) {
		e.state.SetStatus("publish already in progress")
		return
	}
	evt := e.buildEvent()
	if err := evt.Sign(sk); err != nil {
		e.publishing.Store(false)
		e.state.SetStatus("sign event: " + err.Error())
		return
	}
	e.state.SetStatus(fmt.Sprintf("publishing kind %d to %d relays...", evt.Kind, len(relays)))
	go e.publishSignedEvent(evt, relays)
}

func (e *Event) publishSignedEvent(evt *nostr.Event, relays []string) {
	defer e.publishing.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	outcomes, err := e.publisher.Publish(ctx, relays, *evt)
	if err != nil {
		e.state.SetStatus("publish: " + err.Error())
		return
	}
	success := 0
	failures := make([]string, 0)
	for _, outcome := range outcomes {
		if outcome.Err == nil {
			success++
			continue
		}
		failures = append(failures, fmt.Sprintf("%s (%v)", outcome.Relay, outcome.Err))
	}
	status := fmt.Sprintf("published %s (kind %d) to %d/%d relays", evt.ID.Hex(), evt.Kind, success, len(outcomes))
	if len(failures) > 0 {
		status += "; failed: " + strings.Join(failures, ", ")
	}
	e.state.SetStatus(status)
}

func (e *Event) setCurrentTimestamp() {
	now := time.Now().Unix()
	e.createdAtInput.ForceSetValue(strconv.FormatInt(now, 10))
}

func (e *Event) updatePreview() {
	evt := e.buildEvent()
	payload, err := json.MarshalIndent(evt, "", "  ")
	if err != nil {
		e.previewText.SetValue("failed to encode event: " + err.Error())
		return
	}
	e.previewText.SetValue(string(payload))
}

func (e *Event) buildEvent() *nostr.Event {
	evt := &nostr.Event{
		Kind:      nostr.Kind(e.kindInput.Value()),
		CreatedAt: e.parseTimestamp(),
		Content:   e.contentInput.Value(),
		Tags:      e.parseTags(),
	}
	if sk, ok := e.state.SecretKey(); ok {
		evt.PubKey = nostr.GetPublicKey(sk)
	}
	evt.ID = evt.GetID()
	return evt
}

func (e *Event) parseTimestamp() nostr.Timestamp {
	raw := strings.TrimSpace(e.createdAtInput.Value())
	if raw == "" {
		return nostr.Timestamp(time.Now().Unix())
	}
	secs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || secs <= 0 {
		return nostr.Timestamp(time.Now().Unix())
	}
	return nostr.Timestamp(secs)
}

func (e *Event) parseTags() nostr.Tags {
	lines := strings.Split(e.tagsInput.Value(), "\n")
	tags := make(nostr.Tags, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		firstSpace := strings.IndexAny(line, " \t")
		key := line
		rest := ""
		if firstSpace >= 0 {
			key = line[:firstSpace]
			rest = strings.TrimSpace(line[firstSpace+1:])
		}
		key = strings.TrimPrefix(key, "#")
		if key == "" {
			continue
		}
		tag := nostr.Tag{key}
		for _, val := range splitValues(rest) {
			tag = append(tag, val)
		}
		tags = append(tags, tag)
	}
	return tags
}

func splitValues(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

// Relays returns the normalized relay URLs entered by the user.
func (e *Event) Relays() []string {
	lines := strings.Split(e.relaysInput.Value(), "\n")
	relays := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		norm := nostr.NormalizeURL(line)
		if norm == "" {
			continue
		}
		if _, exists := seen[norm]; exists {
			continue
		}
		seen[norm] = struct{}{}
		relays = append(relays, norm)
	}
	return relays
}
