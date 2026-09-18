package frontend

import (
	"image"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// authorizationData is the content model for the shared authorization
// surface. It serves both the standalone authorization shell and the in-app
// Prompt/FocusAuth flow.
type authorizationData struct {
	Label     string
	Guidance  string
	Input     string
	Secret    bool
	Submitted bool
	Waiting   bool
}

// authorizationBoundsUsable reports whether the fixed authorization geometry
// has room for its nine rows and at least one inner content cell. Smaller
// viewports belong to composeTooSmall rather than this shared builder.
func authorizationBoundsUsable(bounds image.Rectangle) bool {
	return bounds.Dx() >= 9 && bounds.Dy() >= 9
}

// authorizationInputRect derives the absolute input field rect from the
// shared authorization frame geometry: a centered frame of width
// min(56, bounds.Dx()-4) and height min(9, bounds.Dy()), with the input row
// two cells inside the left border, one row below the title (frameY+4),
// frameWidth-4 cells wide and exactly one row tall. Unusable bounds return
// an empty rect.
func authorizationInputRect(bounds image.Rectangle) image.Rectangle {
	if !authorizationBoundsUsable(bounds) {
		return image.Rectangle{}
	}
	frameWidth := min(56, bounds.Dx()-4)
	frameHeight := min(9, bounds.Dy())
	frameX := bounds.Min.X + (bounds.Dx()-frameWidth)/2
	frameY := bounds.Min.Y + (bounds.Dy()-frameHeight)/2
	inputX := frameX + 2
	inputY := frameY + 4
	return image.Rect(inputX, inputY, inputX+frameWidth-4, inputY+1)
}

// buildAuthorizationLayer builds the authorization surface as a real rounded
// Style frame root at absolute frame Min with nested content children. It
// returns the surface root plus the absolute cursor when visible. The layer
// has no interactions.
func buildAuthorizationLayer(bounds image.Rectangle, data authorizationData, styles renderStyles, authorizationView ...string) surfaceResult {
	if !authorizationBoundsUsable(bounds) {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	frameWidth := min(56, bounds.Dx()-4)
	frameHeight := min(9, bounds.Dy())
	frameX := bounds.Min.X + (bounds.Dx()-frameWidth)/2
	frameY := bounds.Min.Y + (bounds.Dy()-frameHeight)/2

	frameContent := styles.Panel.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rgba(focusedBorderColor)).
		Width(frameWidth).
		Height(frameHeight).
		Render("")
	frame := lipgloss.NewLayer(frameContent).X(frameX).Y(frameY).Z(zAuthFrame)

	innerWidth := frameWidth - 4
	inputRect := authorizationInputRect(bounds)

	// Title sits on the top border row, exactly like the legacy rounded title.
	// It is clipped to the inner width so it never overwrites the top-right
	// border corner.
	frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Title, "Authorization", max(0, innerWidth))).X(2).Y(0).Z(zAuthContent))

	cursor := renderCursor{X: -1, Y: -1}

	if data.Waiting {
		// Waiting state: no input field or cursor.
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Emphasis, "Connecting to Telegram", innerWidth)).X(2).Y(2).Z(zAuthContent))
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, "Waiting for authorization", innerWidth)).X(2).Y(4).Z(zAuthContent))
	} else if len(authorizationView) > 0 {
		// Production cut-over: embed the injected Huh Input View in the shared
		// input rect. The legacy Prompt.Input display, placeholder and terminal
		// cursor are suppressed; an empty injected view stays empty and never
		// falls back to data.Input.
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Emphasis, data.Label, innerWidth)).X(2).Y(1).Z(zAuthContent))
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, data.Guidance, innerWidth)).X(2).Y(2).Z(zAuthContent))

		frame.AddLayers(lipgloss.NewLayer(clipAuthorizationInputView(authorizationView[0], inputRect.Dx())).X(inputRect.Min.X - frameX).Y(inputRect.Min.Y - frameY).Z(zAuthContent))

		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, "Enter: continue   Backspace: delete   Ctrl-C: quit", innerWidth)).X(2).Y(5).Z(zAuthContent))
		if data.Submitted {
			frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Accent, "Input accepted; waiting for authorization", innerWidth)).X(2).Y(6).Z(zAuthContent))
		}
	} else {
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Emphasis, data.Label, innerWidth)).X(2).Y(1).Z(zAuthContent))
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, data.Guidance, innerWidth)).X(2).Y(2).Z(zAuthContent))

		displayed, cursorOffset := authorizationDisplay(data, innerWidth)
		// The input field removes styles.Input's foreground so its text inherits
		// tea.View.ForegroundColor. With no background rule it uses the terminal's
		// default background while remaining exactly innerWidth cells wide.
		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Input.UnsetForeground(), displayed, inputRect.Dx())).X(inputRect.Min.X - frameX).Y(inputRect.Min.Y - frameY).Z(zAuthContent))

		frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Muted, "Enter: continue   Backspace: delete   Ctrl-C: quit", innerWidth)).X(2).Y(5).Z(zAuthContent))
		if data.Submitted {
			frame.AddLayers(lipgloss.NewLayer(renderLine(styles.Accent, "Input accepted; waiting for authorization", innerWidth)).X(2).Y(6).Z(zAuthContent))
		}

		// Visible cursor at the absolute end of the displayed input, clamped
		// before the inner right edge.
		cursorX := min(inputRect.Min.X+cursorOffset, inputRect.Max.X-1)
		cursor = renderCursor{X: cursorX, Y: inputRect.Min.Y, Visible: true}
	}

	return surfaceResult{
		Layer:   frame,
		Rect:    image.Rect(frameX, frameY, frameX+frameWidth, frameY+frameHeight),
		Cursor:  cursor,
		IsModal: true,
	}
}

// clipAuthorizationInputView restricts an injected Huh authorization View to
// exactly the one-row input rect: the first line only, ANSI-truncated to
// `width` cells so ANSI sequences and wide runes keep their cell width.
// Returns empty string when the view is empty or the width is invalid.
func clipAuthorizationInputView(view string, width int) string {
	if view == "" || width <= 0 {
		return ""
	}
	return ansi.Truncate(strings.Split(view, "\n")[0], width, "")
}

// authorizationDisplay computes the input field display text and the cursor
// offset (in cells) within the input. Empty input shows the placeholder but
// keeps the cursor at offset 0. Secret input masks each rune with a bullet.
func authorizationDisplay(data authorizationData, innerWidth int) (displayed string, cursorOffset int) {
	if data.Secret {
		masked := strings.Repeat("•", utf8.RuneCountInString(data.Input))
		displayed = trailingCells(masked, innerWidth-1)
		cursorOffset = displayWidth(displayed)
		return displayed, cursorOffset
	}
	if data.Input == "" {
		return "Type here…", 0
	}
	displayed = trailingCells(data.Input, innerWidth-1)
	return displayed, displayWidth(displayed)
}

// composeAuthorization is the standalone-auth frame path. It creates one exact
// full-size root using newRenderStyles(false).Base, attaches the authorization
// surface, creates one compositor, renders once, and returns no hits/overlay.
func composeAuthorization(bounds image.Rectangle, data authorizationData) frameResult {
	if !authorizationBoundsUsable(bounds) {
		return frameResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	styles := newRenderStyles(false)
	rootContent := styles.Base.Width(bounds.Dx()).Height(bounds.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	surface := buildAuthorizationLayer(bounds, data, styles)
	root.AddLayers(surface.Layer)

	compositor := lipgloss.NewCompositor(root)
	content := compositor.Render()

	return frameResult{
		Content:    content,
		Hits:       nil,
		Cursor:     surface.Cursor,
		Overlay:    overlayRequest{},
		Compositor: compositor,
	}
}

// composeTooSmall renders a full-size root with a centered warning line for
// viewports below the minimum. It returns no hits/cursor/overlay. Zero or
// negative bounds return an empty frame safely.
func composeTooSmall(bounds image.Rectangle, message string) frameResult {
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return frameResult{Cursor: renderCursor{X: -1, Y: -1}}
	}

	styles := newRenderStyles(false)
	rootContent := styles.Base.Width(bounds.Dx()).Height(bounds.Dy()).Render("")
	root := lipgloss.NewLayer(rootContent).X(0).Y(0).Z(zFrame)

	lineWidth := min(bounds.Dx(), displayWidth(message))
	x := bounds.Min.X + centeredX(bounds.Dx(), lineWidth)
	y := bounds.Min.Y + bounds.Dy()/2
	warning := renderLine(styles.Warning, message, lineWidth)
	root.AddLayers(lipgloss.NewLayer(warning).X(x).Y(y).Z(zAuthContent))

	compositor := lipgloss.NewCompositor(root)
	content := compositor.Render()

	return frameResult{
		Content:    content,
		Hits:       nil,
		Cursor:     renderCursor{X: -1, Y: -1},
		Overlay:    overlayRequest{},
		Compositor: compositor,
	}
}
