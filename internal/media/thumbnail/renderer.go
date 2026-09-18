package thumbnail

import (
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strconv"
	"strings"

	"github.com/blacktop/go-termimg"
	_ "golang.org/x/image/webp"
)

// Block is one rendered thumbnail in terminal-cell coordinates. It is safe to
// store and reuse across frames.
//
// For Halfblocks, Text is a multi-line ANSI half-block string laid out as
// normal text content: Width columns and Height text rows. For Kitty, Text is
// a single kitty graphics transmit escape sequence (an APC with a=T) that must
// be emitted outside the text compositor with the terminal cursor positioned
// at the block's top-left cell; Width and Height describe the cells the image
// overlays. ImageID is the stable kitty image id to reuse across frames and to
// delete when the thumbnail is invalidated.
type Block struct {
	Text    string
	Width   int
	Height  int
	Kitty   bool
	ImageID uint32
}

// Renderer renders a thumbnail file to a display Block.
type Renderer interface {
	// Render renders path to a Block that fits within width by height
	// terminal cells while preserving the source aspect ratio. width and
	// height must be positive; height must be at least 2.
	Render(path string, width, height int) (Block, error)
}

// NewRenderer returns a Renderer backed by go-termimg that sizes thumbnails
// with the library's aspect-correct cell geometry. protocol selects the
// display protocol: termimg.Kitty yields a kitty graphics transmit Block,
// termimg.Halfblocks yields a plain ANSI half-block Block laid out as text.
func NewRenderer(protocol termimg.Protocol) Renderer {
	return widgetRenderer{protocol: protocol}
}

type widgetRenderer struct {
	protocol termimg.Protocol
}

func (r widgetRenderer) Render(path string, width, height int) (Block, error) {
	if path == "" {
		return Block{}, fmt.Errorf("thumbnail: empty path")
	}
	if width < 1 || height < 2 {
		return Block{}, fmt.Errorf("thumbnail: invalid size %dx%d", width, height)
	}
	image, err := termimg.NewImageWidgetFromFile(path)
	if err != nil {
		return Block{}, fmt.Errorf("thumbnail: open image")
	}
	// Correct the requested cell box for the source aspect ratio so the
	// thumbnail keeps the image proportions inside the box.
	image.SetSize(width, height).SetSizeWithCorrection(width, height)
	blockWidth, blockHeight := image.GetSize()
	if blockWidth < 1 {
		blockWidth = 1
	}
	if blockHeight < 1 {
		blockHeight = 1
	}
	if r.protocol == termimg.Kitty {
		image.SetProtocol(termimg.Kitty)
	} else {
		// The half-block renderer consumes pixel dimensions and emits one
		// glyph per 2x2 pixels, so double the corrected cell box.
		image.SetSize(2*blockWidth, 2*blockHeight).SetProtocol(termimg.Halfblocks)
	}
	out, err := image.Render()
	if err != nil {
		return Block{}, fmt.Errorf("thumbnail: render image")
	}
	block := Block{
		Text:   strings.TrimRight(out, "\n"),
		Width:  blockWidth,
		Height: blockHeight,
	}
	if r.protocol == termimg.Kitty {
		block.Kitty = true
		block.ImageID = kittyImageID(out)
	}
	return block, nil
}

var kittyImageIDPattern = regexp.MustCompile(`i=(\d+)`)

// kittyImageID extracts the image id from the first kitty graphics APC
// control segment of a go-termimg transmit string. The scan is limited to the
// first control segment (between \x1b_G and the first ';') so that base64
// payload bytes cannot produce false matches.
func kittyImageID(output string) uint32 {
	const apcPrefix = "\x1b_G"
	start := strings.Index(output, apcPrefix)
	if start < 0 {
		return 0
	}
	control := output[start+len(apcPrefix):]
	if end := strings.IndexByte(control, ';'); end >= 0 {
		control = control[:end]
	}
	match := kittyImageIDPattern.FindStringSubmatch(control)
	if len(match) != 2 {
		return 0
	}
	id, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(id)
}
