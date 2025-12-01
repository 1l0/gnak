package tabs

import (
	"github.com/guigui-gui/guigui"
	"github.com/guigui-gui/guigui/basicwidget"
)

// Placeholder is used while porting the Qt tabs.
type Placeholder struct {
	guigui.DefaultWidget

	text    basicwidget.Text
	message string
}

// NewPlaceholder builds a placeholder tab with the provided message.
func NewPlaceholder(message string) *Placeholder {
	return &Placeholder{message: message}
}

func (p *Placeholder) Build(_ *guigui.Context, adder *guigui.ChildAdder) error {
	adder.AddChild(&p.text)
	p.text.SetValue(p.message)
	p.text.SetAutoWrap(true)
	p.text.SetSelectable(true)
	p.text.SetBold(true)
	return nil
}

func (p *Placeholder) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	u := basicwidget.UnitSize(context)
	layout := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Items: []guigui.LinearLayoutItem{
			{Widget: &p.text},
		},
		Padding: guigui.Padding{Start: u, End: u, Top: u, Bottom: u},
	}
	layout.LayoutWidgets(context, widgetBounds.Bounds(), layouter)
}
