package frontend

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"image"
	"strconv"
	"strings"
)

// clipInlinePlacements clips Kitty thumbnail placements to the clip rectangle,
// cropping partially visible images through a source-rect transmit rewrite.
// Fully visible placements are returned unchanged with their original id; any
// cropped placement gets a stable derived id so the overlay can track it.
func clipInlinePlacements(placements []inlinePlacement, clipRect image.Rectangle) []inlinePlacement {
	return clipInlinePlacementsWithOverlay(placements, clipRect, image.Rectangle{})
}

// clipInlinePlacementsWithOverlay additionally subtracts an opaque overlay
// rectangle (e.g. a modal frame) from each placement. The visible remainder
// decomposes into at most four disjoint horizontal bands, each emitted as its
// own cropped placement with a distinct derived id. Unparseable transmits are
// dropped so a stub can never be drawn at an out-of-bounds position.
func clipInlinePlacementsWithOverlay(placements []inlinePlacement, clipRect, overlay image.Rectangle) []inlinePlacement {
	if len(placements) == 0 {
		return nil
	}
	out := make([]inlinePlacement, 0, len(placements))
	for _, placement := range placements {
		box := image.Rect(placement.X, placement.Y, placement.X+placement.Width, placement.Y+placement.Height)
		visible := box.Intersect(clipRect)
		if visible.Empty() {
			continue
		}
		regions := []image.Rectangle{visible}
		if !overlay.Empty() {
			regions = subtractRect(visible, overlay)
		}
		for index, region := range regions {
			if region == box {
				out = append(out, placement)
				continue
			}
			cropped, ok := cropInlinePlacement(placement, region, derivedInlineID(placement.ImageID, index))
			if ok {
				out = append(out, cropped)
			}
		}
	}
	return out
}

// cropInlinePlacement rewrites a placement so only the absolute visible
// rectangle is drawn. The source image is cropped proportionally to the
// visible cells and the transmit's image id is replaced with imageID. The
// original placement is returned unchanged when visible equals the full box.
func cropInlinePlacement(placement inlinePlacement, visible image.Rectangle, imageID uint32) (inlinePlacement, bool) {
	if visible == image.Rect(placement.X, placement.Y, placement.X+placement.Width, placement.Y+placement.Height) {
		return placement, true
	}
	s, v, ok := parseKittySourceDims(placement.Text)
	if !ok || placement.Width <= 0 || placement.Height <= 0 {
		return inlinePlacement{}, false
	}
	// Existing source rect (after a prior pane clip) defaults to the full
	// image when x/y/w/h are absent.
	curX, curY, curW, curH := parseKittySourceRect(placement.Text, s, v)
	lx := visible.Min.X - placement.X
	ly := visible.Min.Y - placement.Y
	lw := visible.Dx()
	lh := visible.Dy()
	// Map visible cells to source pixels using the current source rect
	// (curW/curH) and current display size (Width/Height), not the original
	// s/v. This keeps aspect when an image is first clipped to the pane
	// and then again to the modal overlay.
	srcX := curX + int(float64(lx)*float64(curW)/float64(placement.Width)+0.5)
	srcY := curY + int(float64(ly)*float64(curH)/float64(placement.Height)+0.5)
	srcW := int(float64(lw)*float64(curW)/float64(placement.Width) + 0.5)
	srcH := int(float64(lh)*float64(curH)/float64(placement.Height) + 0.5)
	srcW = min(s-srcX, min(curW, max(1, srcW)))
	srcH = min(v-srcY, min(curH, max(1, srcH)))
	// Clamp w/h to stay inside the current source rect as well.
	if srcX+srcW > curX+curW {
		srcW = curX + curW - srcX
	}
	if srcY+srcH > curY+curH {
		srcH = curY + curH - srcY
	}
	if srcW <= 0 || srcH <= 0 {
		return inlinePlacement{}, false
	}
	text, err := rewriteKittyTransmit(placement.Text, imageID, lw, lh, srcX, srcY, srcW, srcH)
	if err != nil {
		return inlinePlacement{}, false
	}
	return inlinePlacement{
		ImageID: imageID,
		X:       visible.Min.X,
		Y:       visible.Min.Y,
		Width:   lw,
		Height:  lh,
		Text:    text,
	}, true
}

// subtractRect decomposes base minus cover into disjoint rectangles. With a
// single cover rectangle the remainder is at most four horizontal bands: above,
// below, and the left/right splits of the vertically overlapping middle band.
// image.Rect normalizes negative dimensions, so each band is guarded by an
// explicit positive-size check before construction.
func subtractRect(base, cover image.Rectangle) []image.Rectangle {
	if base.Empty() || cover.Empty() || !base.Overlaps(cover) {
		return []image.Rectangle{base}
	}
	var result []image.Rectangle
	if cover.Min.Y > base.Min.Y {
		result = append(result, image.Rect(base.Min.X, base.Min.Y, base.Max.X, min(base.Max.Y, cover.Min.Y)))
	}
	midMinY := max(base.Min.Y, cover.Min.Y)
	midMaxY := min(base.Max.Y, cover.Max.Y)
	if midMinY < midMaxY {
		if cover.Min.X > base.Min.X {
			result = append(result, image.Rect(base.Min.X, midMinY, min(base.Max.X, cover.Min.X), midMaxY))
		}
		if cover.Max.X < base.Max.X {
			result = append(result, image.Rect(max(base.Min.X, cover.Max.X), midMinY, base.Max.X, midMaxY))
		}
	}
	if cover.Max.Y < base.Max.Y {
		result = append(result, image.Rect(base.Min.X, max(base.Min.Y, cover.Max.Y), base.Max.X, base.Max.Y))
	}
	return result
}

// derivedInlineID derives a stable overlay-tracking image id for a cropped
// placement sub-band from the original image id and the band index.
func derivedInlineID(base uint32, sub int) uint32 {
	hash := fnv.New32a()
	var buffer [8]byte
	binary.LittleEndian.PutUint32(buffer[:4], base)
	binary.LittleEndian.PutUint32(buffer[4:], uint32(sub))
	hash.Write(buffer[:])
	return hash.Sum32()
}

// parseKittySourceDims extracts the s/v source pixel dimensions from the first
// APC control of a cached go-termimg transmit.
func parseKittySourceDims(transmit string) (width, height int, ok bool) {
	control, ok := firstKittyControl(transmit)
	if !ok {
		return 0, 0, false
	}
	for _, token := range strings.Split(control, ",") {
		key, value, cut := strings.Cut(token, "=")
		if !cut {
			continue
		}
		switch key {
		case "s":
			width, _ = strconv.Atoi(value)
		case "v":
			height, _ = strconv.Atoi(value)
		}
	}
	return width, height, width > 0 && height > 0
}

// parseKittySourceRect extracts the current source rectangle x/y/w/h from the
// first APC control, defaulting to 0,0,s,v when absent (first clip).
func parseKittySourceRect(transmit string, s, v int) (x, y, w, h int) {
	control, ok := firstKittyControl(transmit)
	if !ok {
		return 0, 0, s, v
	}
	w, h = s, v
	for _, token := range strings.Split(control, ",") {
		key, value, cut := strings.Cut(token, "=")
		if !cut {
			continue
		}
		switch key {
		case "x":
			x, _ = strconv.Atoi(value)
		case "y":
			y, _ = strconv.Atoi(value)
		case "w":
			w, _ = strconv.Atoi(value)
		case "h":
			h, _ = strconv.Atoi(value)
		}
	}
	if w <= 0 {
		w = s
	}
	if h <= 0 {
		h = v
	}
	return x, y, w, h
}

// rewriteKittyTransmit replaces the image id, the c/r placement cells, and the
// x/y/w/h source rectangle of the first APC control of a cached go-termimg
// transmit, leaving every payload chunk untouched.
func rewriteKittyTransmit(transmit string, imageID uint32, cellsX, cellsY, srcX, srcY, srcW, srcH int) (string, error) {
	idx := strings.Index(transmit, "\x1b_G")
	if idx < 0 {
		return "", errors.New("frontend: no kitty control in transmit")
	}
	semi := strings.Index(transmit[idx:], ";")
	if semi < 0 {
		return "", errors.New("frontend: malformed kitty transmit")
	}
	semi += idx
	control := transmit[idx+3 : semi]
	rewritten := rewriteKittyControl(control, imageID, cellsX, cellsY, srcX, srcY, srcW, srcH)
	return transmit[:idx+3] + rewritten + transmit[semi:], nil
}

// rewriteKittyControl rewrites one kitty control string's key=value tokens in
// place, preserving token order.
func rewriteKittyControl(control string, imageID uint32, cellsX, cellsY, srcX, srcY, srcW, srcH int) string {
	if control == "" {
		return control
	}
	tokens := strings.Split(control, ",")
	out := make([]string, 0, len(tokens)+4)
	foundID := false
	for _, token := range tokens {
		key, _, cut := strings.Cut(token, "=")
		switch {
		case cut && key == "i":
			foundID = true
			out = append(out, "i="+strconv.FormatUint(uint64(imageID), 10))
		case cut && key == "c" && cellsX > 0:
			out = append(out, "c="+strconv.Itoa(cellsX))
		case cut && key == "r" && cellsY > 0:
			out = append(out, "r="+strconv.Itoa(cellsY))
		case cut && (key == "x" || key == "y" || key == "w" || key == "h"):
			// Replaced by the injected source rectangle below.
		default:
			out = append(out, token)
		}
	}
	if !foundID {
		out = append(out, "i="+strconv.FormatUint(uint64(imageID), 10))
	}
	out = append(out,
		"x="+strconv.Itoa(srcX),
		"y="+strconv.Itoa(srcY),
		"w="+strconv.Itoa(srcW),
		"h="+strconv.Itoa(srcH))
	return strings.Join(out, ",")
}

// firstKittyControl returns the control segment (before the first ';') of the
// first APC sequence in a cached go-termimg transmit.
func firstKittyControl(transmit string) (string, bool) {
	idx := strings.Index(transmit, "\x1b_G")
	if idx < 0 {
		return "", false
	}
	semi := strings.Index(transmit[idx:], ";")
	if semi < 0 {
		return "", false
	}
	return transmit[idx+3 : idx+semi], true
}
