package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/guigui-gui/guigui"
	"github.com/guigui-gui/guigui/basicwidget"

	"fiatjaf.com/nostr"

	"github.com/1l0/gnak/internal/nostrutil"
	"github.com/1l0/gnak/internal/state"
)

const (
	maxReqResults    = 400
	timeLayoutRFC333 = time.RFC3339
)

// Req mirrors the original vnak request/filter tab.
type Req struct {
	guigui.DefaultWidget

	state *state.AppState
	pool  *nostr.Pool

	authorsLabel basicwidget.Text
	authorsInput basicwidget.TextInput
	idsLabel     basicwidget.Text
	idsInput     basicwidget.TextInput
	kindsLabel   basicwidget.Text
	kindsInput   basicwidget.TextInput
	tagsLabel    basicwidget.Text
	tagsHelp     basicwidget.Text
	tagsInput    basicwidget.TextInput

	sinceLabel  basicwidget.Text
	sinceToggle basicwidget.Toggle
	sinceInput  basicwidget.TextInput
	sinceNow    basicwidget.Button

	untilLabel  basicwidget.Text
	untilToggle basicwidget.Toggle
	untilInput  basicwidget.TextInput
	untilNow    basicwidget.Button

	limitLabel  basicwidget.Text
	limitToggle basicwidget.Toggle
	limitInput  basicwidget.NumberInput

	filterLabel   basicwidget.Text
	filterPreview basicwidget.Text

	relaysLabel basicwidget.Text
	relaysInput basicwidget.TextInput

	sendButton basicwidget.Button
	stopButton basicwidget.Button

	resultsLabel      basicwidget.Text
	resultsList       basicwidget.List[string]
	resultDetailLabel basicwidget.Text
	resultDetail      basicwidget.Text

	currentFilter nostr.Filter
	lastFilterErr error

	subCancelMu sync.Mutex
	subCancel   *cancelToken
	receiving   atomic.Bool

	resultsMu             sync.Mutex
	results               []nostr.Event
	resultsVersion        atomic.Int64
	appliedResultsVersion int64
	selectedResultID      string
}

type cancelToken struct {
	cancel context.CancelFunc
}

// NewReq creates a req tab bound to the shared nostr pool.
func NewReq(st *state.AppState, pool *nostr.Pool) *Req {
	return &Req{state: st, pool: pool}
}

// Build wires widgets and handlers.
func (r *Req) Build(_ *guigui.Context, adder *guigui.ChildAdder) error {
	for _, widget := range []guigui.Widget{
		&r.authorsLabel,
		&r.authorsInput,
		&r.idsLabel,
		&r.idsInput,
		&r.kindsLabel,
		&r.kindsInput,
		&r.tagsLabel,
		&r.tagsHelp,
		&r.tagsInput,
		&r.sinceLabel,
		&r.sinceToggle,
		&r.sinceInput,
		&r.sinceNow,
		&r.untilLabel,
		&r.untilToggle,
		&r.untilInput,
		&r.untilNow,
		&r.limitLabel,
		&r.limitToggle,
		&r.limitInput,
		&r.filterLabel,
		&r.filterPreview,
		&r.relaysLabel,
		&r.relaysInput,
		&r.sendButton,
		&r.stopButton,
		&r.resultsLabel,
		&r.resultsList,
		&r.resultDetailLabel,
		&r.resultDetail,
	} {
		adder.AddChild(widget)
	}

	r.authorsLabel.SetValue("Authors (hex / npub / nprofile / nip05)")
	r.authorsLabel.SetBold(true)
	r.authorsInput.SetMultiline(true)
	r.authorsInput.SetAutoWrap(true)
	r.authorsInput.SetOnValueChanged(func(string, bool) {
		r.updateFilterPreview()
	})

	r.idsLabel.SetValue("Event IDs (hex / note / nevent)")
	r.idsLabel.SetBold(true)
	r.idsInput.SetMultiline(true)
	r.idsInput.SetAutoWrap(true)
	r.idsInput.SetOnValueChanged(func(string, bool) {
		r.updateFilterPreview()
	})

	r.kindsLabel.SetValue("Kinds (one per line)")
	r.kindsLabel.SetBold(true)
	r.kindsInput.SetMultiline(true)
	r.kindsInput.SetAutoWrap(true)
	r.kindsInput.SetOnValueChanged(func(string, bool) {
		r.updateFilterPreview()
	})

	r.tagsLabel.SetValue("Tags (#e npub ...) — format: key value1 value2 ... or key: value1, value2")
	r.tagsLabel.SetBold(true)
	r.tagsHelp.SetValue("Decoded nip19 values are automatically expanded to their reference form.")
	r.tagsHelp.SetAutoWrap(true)
	r.tagsInput.SetMultiline(true)
	r.tagsInput.SetAutoWrap(true)
	r.tagsInput.SetOnValueChanged(func(string, bool) {
		r.updateFilterPreview()
	})

	r.sinceLabel.SetValue("Since (unix seconds or RFC3339)")
	r.sinceLabel.SetBold(true)
	r.sinceInput.SetMultiline(false)
	r.sinceInput.SetEditable(false)
	r.sinceInput.SetOnValueChanged(func(string, bool) {
		r.updateFilterPreview()
	})
	r.sinceToggle.SetOnValueChanged(func(v bool) {
		r.sinceInput.SetEditable(v)
		r.updateFilterPreview()
	})
	r.sinceNow.SetText("Now")
	r.sinceNow.SetOnUp(func() {
		r.sinceToggle.SetValue(true)
		r.sinceInput.ForceSetValue(strconv.FormatInt(time.Now().Unix(), 10))
		r.sinceInput.SetEditable(true)
		r.updateFilterPreview()
	})

	r.untilLabel.SetValue("Until (unix seconds or RFC3339)")
	r.untilLabel.SetBold(true)
	r.untilInput.SetMultiline(false)
	r.untilInput.SetEditable(false)
	r.untilInput.SetOnValueChanged(func(string, bool) {
		r.updateFilterPreview()
	})
	r.untilToggle.SetOnValueChanged(func(v bool) {
		r.untilInput.SetEditable(v)
		r.updateFilterPreview()
	})
	r.untilNow.SetText("Now")
	r.untilNow.SetOnUp(func() {
		r.untilToggle.SetValue(true)
		r.untilInput.ForceSetValue(strconv.FormatInt(time.Now().Unix(), 10))
		r.untilInput.SetEditable(true)
		r.updateFilterPreview()
	})

	r.limitLabel.SetValue("Limit")
	r.limitLabel.SetBold(true)
	r.limitInput.SetMinimumValue(0)
	r.limitInput.SetMaximumValue(5000)
	r.limitInput.SetValue(100)
	r.limitInput.SetEditable(false)
	r.limitInput.SetOnValueChanged(func(int, bool) {
		r.updateFilterPreview()
	})
	r.limitToggle.SetOnValueChanged(func(v bool) {
		r.limitInput.SetEditable(v)
		r.updateFilterPreview()
	})

	r.filterLabel.SetValue("Filter preview")
	r.filterLabel.SetBold(true)
	r.filterPreview.SetSelectable(true)
	r.filterPreview.SetAutoWrap(true)
	r.filterPreview.SetTabular(true)

	r.relaysLabel.SetValue("Relays (one per line)")
	r.relaysLabel.SetBold(true)
	r.relaysInput.SetMultiline(true)
	r.relaysInput.SetAutoWrap(true)

	r.sendButton.SetText("Send request")
	r.sendButton.SetOnUp(r.handleSend)

	r.stopButton.SetText("Stop")
	r.stopButton.SetOnUp(func() {
		r.state.SetStatus("stopping subscription...")
		r.stopSubscription()
	})

	r.resultsLabel.SetValue("Results")
	r.resultsLabel.SetBold(true)
	r.resultsList.SetStripeVisible(true)
	r.resultsList.SetOnItemSelected(func(index int) {
		item, ok := r.resultsList.ItemByIndex(index)
		if !ok {
			return
		}
		id := item.Value
		r.selectedResultID = id
		r.showResultByID(id)
	})

	r.resultDetailLabel.SetValue("Selected event JSON")
	r.resultDetailLabel.SetBold(true)
	r.resultDetail.SetSelectable(true)
	r.resultDetail.SetAutoWrap(true)
	r.resultDetail.SetTabular(true)
	r.resultDetail.SetValue("results will appear here once events arrive")

	r.updateFilterPreview()
	return nil
}

// Layout arranges the request builder controls.
func (r *Req) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	layout := r.layoutSpec(context)
	layout.LayoutWidgets(context, widgetBounds.Bounds(), layouter)
}

// Measure reports the size of the layout for scroll containers.
func (r *Req) Measure(context *guigui.Context, constraints guigui.Constraints) image.Point {
	layout := r.layoutSpec(context)
	return layout.Measure(context, constraints)
}

func (r *Req) layoutSpec(context *guigui.Context) guigui.LinearLayout {
	u := basicwidget.UnitSize(context)
	sinceRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &r.sinceLabel, Size: guigui.FixedSize(10 * u)},
			{Widget: &r.sinceToggle, Size: guigui.FixedSize(3 * u)},
			{Widget: &r.sinceInput, Size: guigui.FlexibleSize(1)},
			{Widget: &r.sinceNow, Size: guigui.FixedSize(4 * u)},
		},
	}
	untilRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &r.untilLabel, Size: guigui.FixedSize(10 * u)},
			{Widget: &r.untilToggle, Size: guigui.FixedSize(3 * u)},
			{Widget: &r.untilInput, Size: guigui.FlexibleSize(1)},
			{Widget: &r.untilNow, Size: guigui.FixedSize(4 * u)},
		},
	}
	limitRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &r.limitLabel, Size: guigui.FixedSize(10 * u)},
			{Widget: &r.limitToggle, Size: guigui.FixedSize(3 * u)},
			{Widget: &r.limitInput, Size: guigui.FixedSize(8 * u)},
		},
	}
	buttonsRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &r.sendButton, Size: guigui.FixedSize(8 * u)},
			{Widget: &r.stopButton, Size: guigui.FixedSize(6 * u)},
		},
	}

	return guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 2,
		Padding:   guigui.Padding{Start: u, End: u, Top: u, Bottom: u},
		Items: []guigui.LinearLayoutItem{
			{Widget: &r.authorsLabel},
			{Widget: &r.authorsInput, Size: guigui.FixedSize(4 * u)},
			{Widget: &r.idsLabel},
			{Widget: &r.idsInput, Size: guigui.FixedSize(3 * u)},
			{Widget: &r.kindsLabel},
			{Widget: &r.kindsInput, Size: guigui.FixedSize(3 * u)},
			{Widget: &r.tagsLabel},
			{Widget: &r.tagsHelp},
			{Widget: &r.tagsInput, Size: guigui.FixedSize(3 * u)},
			{Layout: sinceRow},
			{Layout: untilRow},
			{Layout: limitRow},
			{Widget: &r.filterLabel},
			{Widget: &r.filterPreview, Size: guigui.FixedSize(4 * u)},
			{Widget: &r.relaysLabel},
			{Widget: &r.relaysInput, Size: guigui.FixedSize(3 * u)},
			{Layout: buttonsRow},
			{Widget: &r.resultsLabel},
			{Widget: &r.resultsList, Size: guigui.FlexibleSize(2)},
			{Widget: &r.resultDetailLabel},
			{Widget: &r.resultDetail, Size: guigui.FlexibleSize(1)},
		},
	}
}

// Tick keeps the UI synced with background activity.
func (r *Req) Tick(context *guigui.Context, _ *guigui.WidgetBounds) error {
	context.SetEnabled(&r.stopButton, r.receiving.Load())
	if version := r.resultsVersion.Load(); version != r.appliedResultsVersion {
		r.refreshResults()
		r.appliedResultsVersion = version
	}
	return nil
}

func (r *Req) handleSend() {
	filter, err := r.rebuildFilter()
	if err != nil {
		r.state.SetStatus("invalid filter: " + err.Error())
		return
	}
	relays := normalizeRelays(r.relaysInput.Value())
	if len(relays) == 0 {
		r.state.SetStatus("add at least one relay URL")
		return
	}
	if r.pool == nil {
		r.state.SetStatus("nostr pool not initialized")
		return
	}
	for _, relay := range relays {
		if relay == "" {
			r.state.SetStatus("relay list contains empty entries")
			return
		}
	}
	r.startSubscription(filter, relays)
}

func (r *Req) startSubscription(filter nostr.Filter, relays []string) {
	r.stopSubscription()
	ctx, cancel := context.WithCancel(context.Background())
	r.subCancelMu.Lock()
	token := &cancelToken{cancel: cancel}
	r.subCancel = token
	r.subCancelMu.Unlock()
	r.resultsVersion.Store(0)
	r.appliedResultsVersion = 0
	r.selectedResultID = ""
	r.resultsMu.Lock()
	r.results = r.results[:0]
	r.resultsMu.Unlock()
	r.receiving.Store(true)
	guigui.RequestRedraw(r)
	r.state.SetStatus(fmt.Sprintf("subscribing to %s", strings.Join(nostrutil.NiceRelayURLs(relays), ", ")))
	go r.runSubscription(ctx, filter, relays, token)
}

func (r *Req) runSubscription(ctx context.Context, filter nostr.Filter, relays []string, token *cancelToken) {
	defer func() {
		token.cancel()
		r.subCancelMu.Lock()
		if r.subCancel == token {
			r.subCancel = nil
		}
		r.subCancelMu.Unlock()
		r.receiving.Store(false)
		r.state.SetStatus("subscription stopped")
		guigui.RequestRedraw(r)
	}()

	if len(relays) == 1 {
		r.subscribeSingle(ctx, relays[0], filter)
		return
	}
	r.subscribeMany(ctx, relays, filter)
}

func (r *Req) subscribeSingle(ctx context.Context, relayURL string, filter nostr.Filter) {
	relay, err := r.pool.EnsureRelay(relayURL)
	if err != nil {
		r.state.SetStatus(fmt.Sprintf("connect %s: %v", nostrutil.NiceRelayURL(relayURL), err))
		return
	}
	sub, err := relay.Subscribe(ctx, filter, nostr.SubscriptionOptions{Label: "gnak-req"})
	if err != nil {
		r.state.SetStatus(fmt.Sprintf("subscribe %s: %v", nostrutil.NiceRelayURL(relay.URL), err))
		return
	}
	r.state.SetStatus(fmt.Sprintf("subscribed to %s", nostrutil.NiceRelayURL(relay.URL)))
	go r.watchEOSE(ctx, sub.EndOfStoredEvents, []string{relay.URL})
	go r.watchClosedReason(ctx, sub.ClosedReason)
	r.consumeEvents(ctx, sub.Events)
}

func (r *Req) subscribeMany(ctx context.Context, relays []string, filter nostr.Filter) {
	eoseChan := make(chan struct{})
	stream := r.pool.SubscribeManyNotifyEOSE(ctx, relays, filter, eoseChan, nostr.SubscriptionOptions{Label: "gnak-req"})
	go r.watchEOSE(ctx, eoseChan, relays)
	for {
		select {
		case <-ctx.Done():
			return
		case ie, ok := <-stream:
			if !ok {
				r.state.SetStatus("subscription ended")
				return
			}
			r.handleIncomingEvent(ie.Event)
		}
	}
}

func (r *Req) watchClosedReason(ctx context.Context, ch <-chan string) {
	select {
	case <-ctx.Done():
	case reason, ok := <-ch:
		if ok && reason != "" {
			r.state.SetStatus("subscription closed: " + reason)
		}
	}
}

func (r *Req) watchEOSE(ctx context.Context, ch <-chan struct{}, relays []string) {
	select {
	case <-ctx.Done():
	case _, ok := <-ch:
		if ok {
			r.state.SetStatus(fmt.Sprintf("EOSE reached for %s; streaming live events", strings.Join(nostrutil.NiceRelayURLs(relays), ", ")))
		}
	}
}

func (r *Req) consumeEvents(ctx context.Context, events <-chan nostr.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				r.state.SetStatus("subscription ended")
				return
			}
			r.handleIncomingEvent(evt)
		}
	}
}

func (r *Req) stopSubscription() {
	r.subCancelMu.Lock()
	token := r.subCancel
	r.subCancel = nil
	r.subCancelMu.Unlock()
	if token != nil {
		token.cancel()
	}
}

func (r *Req) handleIncomingEvent(evt nostr.Event) {
	r.resultsMu.Lock()
	defer r.resultsMu.Unlock()
	if len(r.results) >= maxReqResults {
		r.results = r.results[:maxReqResults-1]
	}
	r.results = append([]nostr.Event{evt}, r.results...)
	r.resultsVersion.Add(1)
	guigui.RequestRedraw(r)
}

func (r *Req) refreshResults() {
	r.resultsMu.Lock()
	items := make([]basicwidget.ListItem[string], len(r.results))
	for i, evt := range r.results {
		timestamp := time.Unix(int64(evt.CreatedAt), 0).UTC().Format(time.DateTime)
		summary := fmt.Sprintf("%s | kind %d | %s", timestamp, evt.Kind, evt.PubKey.Hex())
		items[i] = basicwidget.ListItem[string]{
			Text:  summary,
			Value: evt.ID.Hex(),
		}
	}
	selectedID := r.selectedResultID
	selectFirst := false
	if selectedID == "" && len(items) > 0 {
		selectedID = items[0].Value
		r.selectedResultID = selectedID
		selectFirst = true
	}
	r.resultsMu.Unlock()

	r.resultsList.SetItems(items)
	if selectedID != "" {
		if selectFirst {
			r.resultsList.SelectItemByIndex(0)
		} else {
			r.resultsList.SelectItemByValue(selectedID)
		}
		r.showResultByID(selectedID)
		return
	}
	r.resultDetail.SetValue("results will appear here once events arrive")
}

func (r *Req) showResultByID(id string) {
	r.resultsMu.Lock()
	defer r.resultsMu.Unlock()
	for _, evt := range r.results {
		if evt.ID.Hex() != id {
			continue
		}
		pretty, err := json.MarshalIndent(evt, "", "  ")
		if err != nil {
			r.resultDetail.SetValue("failed to encode event: " + err.Error())
			return
		}
		r.resultDetail.SetValue(string(pretty))
		return
	}
}

func (r *Req) rebuildFilter() (nostr.Filter, error) {
	var filter nostr.Filter

	if authors := parseAuthors(r.authorsInput.Value()); len(authors) > 0 {
		filter.Authors = authors
	}
	if ids := parseIDs(r.idsInput.Value()); len(ids) > 0 {
		filter.IDs = ids
	}
	if kinds, err := parseKinds(r.kindsInput.Value()); err != nil {
		return filter, err
	} else if len(kinds) > 0 {
		filter.Kinds = kinds
	}
	if tags := parseTags(r.tagsInput.Value()); len(tags) > 0 {
		filter.Tags = tags
	}

	if r.sinceToggle.Value() {
		ts, err := parseTimestamp(r.sinceInput.Value())
		if err != nil {
			return filter, fmt.Errorf("invalid since: %w", err)
		}
		filter.Since = ts
	}
	if r.untilToggle.Value() {
		ts, err := parseTimestamp(r.untilInput.Value())
		if err != nil {
			return filter, fmt.Errorf("invalid until: %w", err)
		}
		filter.Until = ts
	}

	if r.limitToggle.Value() {
		limit := r.limitInput.Value()
		if limit > 0 {
			filter.Limit = limit
			filter.LimitZero = false
		} else {
			filter.Limit = 0
			filter.LimitZero = true
		}
	}

	return filter, nil
}

func (r *Req) updateFilterPreview() {
	filter, err := r.rebuildFilter()
	if err != nil {
		r.filterPreview.SetValue("filter error: " + err.Error())
		r.currentFilter = nostr.Filter{}
		r.lastFilterErr = err
		return
	}
	r.currentFilter = filter
	r.lastFilterErr = nil
	payload, _ := json.MarshalIndent(filter, "", "  ")
	r.filterPreview.SetValue(string(payload))
}

func parseAuthors(text string) []nostr.PubKey {
	lines := strings.Split(text, "\n")
	pubkeys := make([]nostr.PubKey, 0, len(lines))
	for _, line := range lines {
		pk, err := nostrutil.ParsePubKey(line)
		if err == nil {
			pubkeys = append(pubkeys, pk)
		}
	}
	return pubkeys
}

func parseIDs(text string) []nostr.ID {
	lines := strings.Split(text, "\n")
	ids := make([]nostr.ID, 0, len(lines))
	for _, line := range lines {
		id, err := nostrutil.ParseEventID(line)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseKinds(text string) ([]nostr.Kind, error) {
	lines := strings.Split(text, "\n")
	kinds := make([]nostr.Kind, 0, len(lines))
	for _, line := range lines {
		fields := strings.FieldsFunc(line, func(r rune) bool {
			return unicode.IsSpace(r) || r == ',' || r == ';'
		})
		for _, field := range fields {
			value := strings.TrimSpace(field)
			if value == "" {
				continue
			}
			k, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid kind %q", value)
			}
			kinds = append(kinds, nostr.Kind(k))
		}
	}
	return kinds, nil
}

func parseTags(text string) nostr.TagMap {
	lines := strings.Split(text, "\n")
	tags := make(nostr.TagMap)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key := line
		valuesPart := ""
		if idx := strings.Index(line, ":"); idx >= 0 {
			key = line[:idx]
			valuesPart = line[idx+1:]
		} else {
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			key = parts[0]
			valuesPart = strings.Join(parts[1:], " ")
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "#"))
		if key == "" {
			continue
		}
		values := splitTagValues(valuesPart)
		if len(values) == 0 {
			continue
		}
		decoded := make([]string, 0, len(values))
		for _, val := range values {
			decoded = append(decoded, nostrutil.DecodeTagValue(val))
		}
		tags[key] = decoded
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func splitTagValues(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';'
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parseTimestamp(text string) (nostr.Timestamp, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nostr.Timestamp(time.Now().Unix()), nil
	}
	if secs, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return nostr.Timestamp(secs), nil
	}
	if t, err := time.Parse(timeLayoutRFC333, trimmed); err == nil {
		return nostr.Timestamp(t.Unix()), nil
	}
	return 0, fmt.Errorf("expected unix seconds or RFC3339 timestamp")
}

func normalizeRelays(text string) []string {
	lines := strings.Split(text, "\n")
	seen := make(map[string]struct{}, len(lines))
	relays := make([]string, 0, len(lines))
	for _, line := range lines {
		norm := nostr.NormalizeURL(strings.TrimSpace(line))
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		relays = append(relays, norm)
	}
	return relays
}
