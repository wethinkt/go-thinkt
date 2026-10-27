//go:build ignore

// Run from the neighboring nthtml module to exercise its local APIs without
// introducing its dependencies into thinkt:
// go -C ../nthtml run ../go-thinkt/etc/conversation-demo.go -mode view FILE.html
// Prefer sh etc/try-conversation.sh for current local checkouts: it uses the
// upstream CLI and a temporary workspace for unpublished nthtml/nttypeset commits.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nthtml/typeset"
	"nimbleterminal.dev/nttypeset"
)

func main() {
	mode := flag.String("mode", "view", "view, tty, png (native nthtml), or typeset (bridge comparison)")
	width := flag.Int("width", 800, "raster width in pixels")
	columns := flag.Int("columns", 80, "TTY width in cells")
	output := flag.String("o", "/tmp/thinkt-conversation.png", "PNG output file")
	flag.Parse()
	if flag.NArg() != 1 || *width < 1 || *columns < 2 {
		fmt.Fprintln(os.Stderr, "usage: conversation-demo [-mode view|tty|png|typeset] [-width pixels] [-columns cells] [-o image.png] FILE.html")
		os.Exit(2)
	}
	if err := run(*mode, flag.Arg(0), *output, *width, *columns); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(mode, path, output string, width, columns int) error {
	ctx := context.Background()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	doc, err := document.Parse(ctx, f, document.Limits{})
	if err != nil {
		return err
	}
	switch mode {
	case "tty":
		text, err := doc.TTY(ctx, columns)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, text)
		return err
	case "typeset":
		styled, sources, err := typeset.BuildWithSources(ctx, doc, typeset.Options{Document: document.Options{Width: width}})
		if err != nil {
			return err
		}
		engine, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
		if err != nil {
			return err
		}
		layout, err := engine.Layout(styled, width, nttypeset.LayoutOptions{})
		if err != nil {
			return err
		}
		img, err := layout.Raster(ctx, nttypeset.View{Width: width, Height: 1000, Scale: 1}, nttypeset.RasterOptions{Palette: sources.Palette()})
		if err != nil {
			return err
		}
		return writePNG(output, img)
	case "png", "view":
		result, err := doc.Render(ctx, document.Options{Width: width, NoImage: mode == "view"})
		if err != nil {
			return err
		}
		if mode == "png" {
			return writePNG(output, result.Image)
		}
		host := &viewer{model: nthtml.New(result), columns: columns}
		defer func() { _ = host.model.Close() }()
		_, err = tea.NewProgram(host).Run()
		return err
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}

type viewer struct {
	model   *nthtml.Model
	columns int
	closing bool
}

type shutdownReady struct{}

func (v *viewer) Init() tea.Cmd { return v.model.Init() }

func (v *viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.closing {
		switch msg.(type) {
		case shutdownReady:
			return v, tea.Quit
		case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg, tea.WindowSizeMsg:
			return v, nil
		default:
			return v, v.model.Update(msg)
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.columns = msg.Width
		return v, tea.Batch(v.model.SetBounds(0, 0, msg.Width, max(1, msg.Height-1)), v.model.Update(msg))
	case tea.KeyPressMsg:
		if !v.model.SearchInputActive() && (msg.String() == "q" || msg.String() == "ctrl+c") {
			v.closing = true
			return v, tea.Sequence(v.model.Close(), tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return shutdownReady{} }))
		}
	case nthtml.CopyMsg:
		return v, tea.SetClipboard(msg.Text)
	}
	return v, v.model.Update(msg)
}

func (v *viewer) View() tea.View {
	help := "t: TTY/typeset · /: search · n/N: matches · drag/y: copy · Tab/Enter: anchors · q: quit"
	view := tea.NewView(v.model.View() + "\n" + ansi.Truncate(help, max(1, v.columns), "…"))
	view.AltScreen = true
	if !v.closing {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
