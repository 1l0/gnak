package ui

import (
	"image/color"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/guigui-gui/guigui"
	"github.com/guigui-gui/guigui/basicwidget"
	"github.com/guigui-gui/guigui/basicwidget/cjkfont"
	"golang.org/x/text/language"

	"github.com/1l0/gnak/internal/event"
	"github.com/1l0/gnak/internal/state"
	"github.com/1l0/gnak/internal/ui/tabs"
)

// Root composes the overall application layout.
type Root struct {
	guigui.DefaultWidget

	state *state.AppState

	background   basicwidget.Background
	secretLabel  basicwidget.Text
	secretInput  basicwidget.TextInput
	secretStatus basicwidget.Text
	generateKey  basicwidget.Button
	statusLabel  basicwidget.Text
	tabList      basicwidget.List[state.TabKind]
	tabs         []tabEntry
	tabView      tabView

	locales     []language.Tag
	faceSources []basicwidget.FaceSourceEntry

	lastSecretErr string
}

type tabEntry struct {
	kind   state.TabKind
	title  string
	widget guigui.Widget
	panel  *basicwidget.Panel
}

// NewRoot wires the shared state into a Guigui widget tree.
func NewRoot(st *state.AppState) *Root {
	r := &Root{state: st}
	pool := nostr.NewPool(nostr.PoolOptions{PenaltyBox: true})
	eventPublisher := event.NewPublisher(pool)
	r.tabs = []tabEntry{
		newTabEntry(state.TabEvent, "event", tabs.NewEvent(st, eventPublisher)),
		newTabEntry(state.TabReq, "req", tabs.NewReq(st, pool)),
		newTabEntry(state.TabPaste, "paste", tabs.NewPaste(st)),
		newTabEntry(state.TabServe, "serve", tabs.NewPlaceholder("Serve tab coming soon")),
	}
	r.tabView.state = st
	r.tabView.tabs = r.tabs
	return r
}

func newTabEntry(kind state.TabKind, title string, widget guigui.Widget) tabEntry {
	panel := &basicwidget.Panel{}
	panel.SetContent(widget)
	panel.SetAutoBorder(true)
	panel.SetContentConstraints(basicwidget.PanelContentConstraintsFixedWidth)
	panel.SetStyle(basicwidget.PanelStyleSide)
	return tabEntry{kind: kind, title: title, widget: widget, panel: panel}
}

// Build registers child widgets and event handlers.
func (r *Root) Build(context *guigui.Context, adder *guigui.ChildAdder) error {
	adder.AddChild(&r.background)
	adder.AddChild(&r.secretLabel)
	adder.AddChild(&r.secretInput)
	adder.AddChild(&r.secretStatus)
	adder.AddChild(&r.generateKey)
	adder.AddChild(&r.statusLabel)
	adder.AddChild(&r.tabList)
	adder.AddChild(&r.tabView)

	r.secretLabel.SetValue("Private key (hex or nsec)")
	r.secretLabel.SetBold(true)
	r.secretStatus.SetColor(color.RGBA{R: 0xC4, G: 0x1B, B: 0x1B, A: 0xFF})
	r.secretStatus.SetAutoWrap(true)
	r.secretStatus.SetSelectable(true)

	r.secretInput.SetMultiline(false)
	r.secretInput.SetOnValueChanged(func(text string, _ bool) {
		if err := r.state.SetSecretInput(text); err != nil {
			r.lastSecretErr = err.Error()
			r.secretStatus.SetValue("Invalid secret: " + err.Error())
		} else {
			r.lastSecretErr = ""
			r.secretStatus.SetValue("")
		}
	})

	r.generateKey.SetText("Generate")
	r.generateKey.SetOnUp(func() {
		sk := nostr.Generate()
		nsec := nip19.EncodeNsec(sk)
		r.secretInput.ForceSetValue(nsec)
		_ = r.state.SetSecretInput(nsec)
		r.secretStatus.SetValue("Generated a brand new key")
	})

	r.statusLabel.SetAutoWrap(true)
	r.statusLabel.SetSelectable(true)

	listItems := make([]basicwidget.ListItem[state.TabKind], len(r.tabs))
	for i := range r.tabs {
		listItems[i] = basicwidget.ListItem[state.TabKind]{
			Text:  strings.ToUpper(r.tabs[i].title),
			Value: r.tabs[i].kind,
		}
	}
	r.tabList.SetStyle(basicwidget.ListStyleSidebar)
	r.tabList.SetStripeVisible(false)
	r.tabList.SetItemHeight(basicwidget.UnitSize(context))
	r.tabList.SetItems(listItems)
	r.tabList.SelectItemByValue(r.state.SelectedTab())
	r.tabList.SetOnItemSelected(func(index int) {
		item, ok := r.tabList.ItemByIndex(index)
		if !ok {
			return
		}
		r.state.SetSelectedTab(item.Value)
		r.statusLabel.SetValue("")
		guigui.RequestRedraw(r)
		guigui.RequestRedraw(&r.tabView)
	})

	r.locales = context.AppendLocales(r.locales[:0])
	r.faceSources = cjkfont.AppendRecommendedFaceSourceEntries(r.faceSources[:0], r.locales)
	basicwidget.SetFaceSources(r.faceSources)

	if current := r.state.SecretInput(); current != "" {
		r.secretInput.ForceSetValue(current)
	}

	return nil
}

// Layout arranges the high-level widgets.
func (r *Root) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	layouter.LayoutWidget(&r.background, widgetBounds.Bounds())

	u := basicwidget.UnitSize(context)
	contentRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u,
		Items: []guigui.LinearLayoutItem{
			{Widget: &r.tabList, Size: guigui.FixedSize(8 * u)},
			{Widget: &r.tabView, Size: guigui.FlexibleSize(1)},
		},
	}

	layout := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 2,
		Padding: guigui.Padding{
			Start:  u,
			End:    u,
			Top:    u,
			Bottom: u,
		},
		Items: []guigui.LinearLayoutItem{
			{
				Layout: guigui.LinearLayout{
					Direction: guigui.LayoutDirectionHorizontal,
					Gap:       u / 2,
					Items: []guigui.LinearLayoutItem{
						{Widget: &r.secretLabel, Size: guigui.FixedSize(8 * u)},
						{Widget: &r.secretInput, Size: guigui.FlexibleSize(1)},
						{Widget: &r.generateKey, Size: guigui.FixedSize(7 * u)},
					},
				},
			},
			{Widget: &r.secretStatus},
			{Layout: contentRow, Size: guigui.FlexibleSize(1)},
			{Widget: &r.statusLabel},
		},
	}

	layout.LayoutWidgets(context, widgetBounds.Bounds(), layouter)
}

// Tick keeps the UI in sync with shared state.
func (r *Root) Tick(context *guigui.Context, widgetBounds *guigui.WidgetBounds) error {
	r.tabList.SelectItemByValue(r.state.SelectedTab())

	r.statusLabel.SetValue(r.state.Status())
	if errText := r.lastSecretErr; errText != "" {
		r.secretStatus.SetValue("Invalid secret: " + errText)
	}
	return nil
}

type tabView struct {
	guigui.DefaultWidget

	state        *state.AppState
	tabs         []tabEntry
	lastSelected state.TabKind
}

func (t *tabView) Build(_ *guigui.Context, adder *guigui.ChildAdder) error {
	for i := range t.tabs {
		adder.AddChild(t.tabs[i].panel)
	}
	return nil
}

func (t *tabView) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	bounds := widgetBounds.Bounds()
	selected := t.state.SelectedTab()
	for i := range t.tabs {
		layouter.LayoutWidget(t.tabs[i].panel, bounds)
		context.SetVisible(t.tabs[i].panel, t.tabs[i].kind == selected)
	}
}

func (t *tabView) Tick(context *guigui.Context, _ *guigui.WidgetBounds) error {
	selected := t.state.SelectedTab()
	if selected != t.lastSelected {
		for i := range t.tabs {
			if t.tabs[i].kind == selected {
				t.tabs[i].panel.SetScrollOffset(0, 0)
				break
			}
		}
		t.lastSelected = selected
	}
	for i := range t.tabs {
		context.SetVisible(t.tabs[i].panel, t.tabs[i].kind == selected)
	}
	return nil
}
