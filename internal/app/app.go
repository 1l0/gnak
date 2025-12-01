package app

import (
	"fmt"
	"image"
	"os"

	"github.com/guigui-gui/guigui"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/1l0/gnak/internal/state"
	"github.com/1l0/gnak/internal/ui"
)

// Run bootstraps Guigui and starts the rendering/game loop.
func Run() error {
	appState := state.New()
	root := ui.NewRoot(appState)

	opts := &guigui.RunOptions{
		Title:      "gnak",
		WindowSize: image.Pt(960, 720),
		RunGameOptions: &ebiten.RunGameOptions{
			ApplePressAndHoldEnabled: true,
		},
	}

	if err := guigui.Run(root, opts); err != nil {
		return fmt.Errorf("run guigui: %w", err)
	}
	fmt.Fprintln(os.Stdout, "gnak exited")
	return nil
}
