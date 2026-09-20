package frontend

import (
	"fmt"
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// displayWidth returns the display width in cells of s, treating the text as a
// sequence of grapheme clusters.
func displayWidth(s string) int {
	return ansi.StringWidth(s)
}

// clipLine normalizes whitespace exactly like the legacy clipSurfaceLine and
// then truncates the value to the given width, appending an ellipsis when
// truncation is required. A width of 1 returns "…" when truncation is needed.
func clipLine(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if width <= 0 || s == "" {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

// trailingCells returns the trailing portion of s that fits within the given
// width, measured in cells. It never splits a grapheme cluster.
func trailingCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	total := ansi.StringWidth(s)
	if total <= width {
		return s
	}

	// Walk grapheme clusters from the end, retaining each cluster only while
	// it still fits within the remaining width. This keeps a whole wide
	// grapheme out when the left boundary would fall inside it.
	type cluster struct {
		text  string
		width int
	}
	var clusters []cluster
	remaining := s
	for remaining != "" {
		text, clusterWidth := ansi.FirstGraphemeCluster(remaining, ansi.GraphemeWidth)
		clusters = append(clusters, cluster{text: text, width: clusterWidth})
		remaining = remaining[len(text):]
	}

	// Collect the retained cluster texts from the end, then join them in
	// forward order so the suffix preserves the original cluster sequence.
	var kept []string
	used := 0
	for i := len(clusters) - 1; i >= 0; i-- {
		if used+clusters[i].width > width {
			break
		}
		used += clusters[i].width
		kept = append(kept, clusters[i].text)
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return strings.Join(kept, "")
}

// wrapText preserves explicit newlines and wraps content to the given width,
// measuring each grapheme cluster. A single cluster wider than the line
// becomes "…" so the line always fits.
func wrapText(s string, width int) []string {
	if width <= 0 || s == "" {
		return nil
	}
	result := make([]string, 0, 2)
	for _, logical := range strings.Split(s, "\n") {
		if logical == "" {
			result = append(result, "")
			continue
		}
		var line strings.Builder
		used := 0
		remaining := logical
		for remaining != "" {
			cluster, clusterWidth := ansi.FirstGraphemeCluster(remaining, ansi.GraphemeWidth)
			remaining = remaining[len(cluster):]
			if used > 0 && used+clusterWidth > width {
				result = append(result, line.String())
				line.Reset()
				used = 0
			}
			if clusterWidth > width {
				// A single cluster wider than the line cannot fit; emit an
				// ellipsis and continue with the remaining input.
				line.WriteString("…")
				used++
				continue
			}
			line.WriteString(cluster)
			used += clusterWidth
		}
		result = append(result, line.String())
	}
	return result
}

// renderLine renders a single fixed-width line. The text is first truncated
// to the width so it never wraps onto a second row.
func renderLine(style lipgloss.Style, text string, width int) string {
	clipped := ansi.Truncate(text, width, "")
	return style.Width(width).Height(1).Render(clipped)
}

// renderEmptyBox renders an empty fixed-size box of the given dimensions.
func renderEmptyBox(style lipgloss.Style, width, height int) string {
	return style.Width(width).Height(height).Render("")
}

// centeredX returns the left coordinate that centers contentWidth within
// containerWidth.
func centeredX(containerWidth, contentWidth int) int {
	return max(0, (containerWidth-contentWidth)/2)
}

// addInteractive creates a fixed-size visual interactive child layer and the
// matching interaction from a single geometry source. The returned
// layerInteraction carries the absolute viewport rectangle (local plus the
// accumulated parent origin) with dimensions identical to local.
//
// When content is non-empty it must already be a fully-formed fixed-size
// string whose Lipgloss width/height exactly match local.Dx()/Dy(); a
// mismatch is a programmer error and panics. Empty content renders an empty
// fixed-size box. An empty local rectangle produces a safe empty interaction
// without adding any layer.
func addInteractive(
	parent *lipgloss.Layer,
	parentOrigin image.Point,
	local image.Rectangle,
	id string,
	z int,
	content string,
	click, wheelUp, wheelDown ActionReceived,
) layerInteraction {
	width := local.Dx()
	height := local.Dy()
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	interaction := layerInteraction{
		ID:        id,
		Rect:      local.Add(parentOrigin),
		Z:         z,
		Click:     click,
		WheelUp:   wheelUp,
		WheelDown: wheelDown,
		Primary:   click,
	}
	if width == 0 || height == 0 {
		return interaction
	}

	if content != "" {
		// The caller supplies a fully-formed fixed-size content string. It must
		// match the target dimensions exactly; restyling or padding arbitrary
		// ANSI content to fit would silently corrupt the layer geometry.
		if lipgloss.Width(content) != width || lipgloss.Height(content) != height {
			panic(fmt.Sprintf(
				"addInteractive: content for %q is %dx%d but local rectangle is %dx%d",
				id, lipgloss.Width(content), lipgloss.Height(content), width, height,
			))
		}
		layer := lipgloss.NewLayer(content).ID(id).X(local.Min.X).Y(local.Min.Y).Z(z)
		parent.AddLayers(layer)
		return interaction
	}

	fixed := renderEmptyBox(lipgloss.NewStyle(), width, height)
	layer := lipgloss.NewLayer(fixed).ID(id).X(local.Min.X).Y(local.Min.Y).Z(z)
	parent.AddLayers(layer)
	return interaction
}
