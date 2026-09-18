package thumbnail

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blacktop/go-termimg"
)

func TestMain(m *testing.M) {
	// Keep go-termimg from querying the terminal during detection while the
	// renderer forces its protocol explicitly.
	os.Setenv("TERMIMG_BYPASS_DETECTION", "kitty")
	os.Exit(m.Run())
}

func writeTestPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: 200, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "test.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestWebP(t *testing.T) string {
	t.Helper()
	const encoded = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sticker.webp")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderStickerWebP(t *testing.T) {
	path := writeTestWebP(t)
	for _, protocol := range []termimg.Protocol{termimg.Halfblocks, termimg.Kitty} {
		block, err := NewRenderer(protocol).Render(path, 12, 6)
		if err != nil {
			t.Fatalf("protocol %v: render WebP: %v", protocol, err)
		}
		if block.Width <= 0 || block.Height <= 0 || block.Text == "" {
			t.Fatalf("protocol %v: invalid WebP block: %#v", protocol, block)
		}
	}
}

func TestRenderProducesBlockGeometry(t *testing.T) {
	r := NewRenderer(termimg.Halfblocks)
	path := writeTestPNG(t, 64, 64)
	b, err := r.Render(path, 20, 16)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if b.Width != 20 {
		t.Errorf("Width = %d, want 20", b.Width)
	}
	if b.Height != 10 {
		t.Errorf("Height = %d, want 10", b.Height)
	}
	if n := strings.Count(b.Text, "\n") + 1; n != 10 {
		t.Errorf("text rows = %d (via newline count), want 10", n)
	}
	if b.Kitty {
		t.Error("half-block render marked the block as Kitty")
	}
}

func TestRenderAspectCorrectsWideImage(t *testing.T) {
	r := NewRenderer(termimg.Halfblocks)
	path := writeTestPNG(t, 64, 32)
	b, err := r.Render(path, 20, 16)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if b.Width != 20 {
		t.Errorf("Width = %d, want 20", b.Width)
	}
	if b.Height != 5 {
		t.Errorf("Height = %d, want 5 for a 2:1 source in a 20x16 box", b.Height)
	}
}

func TestRenderANSIOutput(t *testing.T) {
	r := NewRenderer(termimg.Halfblocks)
	path := writeTestPNG(t, 64, 64)
	b, err := r.Render(path, 20, 16)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if b.Text == "" {
		t.Fatal("Text is empty")
	}
	if !strings.Contains(b.Text, "\x1b[") {
		t.Fatal("Text does not contain ANSI escape sequences")
	}
}

func TestRenderKittyBlock(t *testing.T) {
	r := NewRenderer(termimg.Kitty)
	path := writeTestPNG(t, 64, 64)
	b, err := r.Render(path, 20, 16)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !b.Kitty {
		t.Fatal("Kitty render did not mark the block as Kitty")
	}
	if b.Width != 20 || b.Height != 10 {
		t.Errorf("geometry = %dx%d, want 20x10", b.Width, b.Height)
	}
	if b.ImageID == 0 {
		t.Fatal("Kitty render did not extract a nonzero image id")
	}
	if !strings.Contains(b.Text, "\x1b_G") {
		t.Fatal("Kitty render text has no graphics transmit")
	}
	if strings.HasSuffix(b.Text, "\n") {
		t.Fatal("Kitty render text retains a trailing newline")
	}
}

func TestRenderEmptyPathError(t *testing.T) {
	r := NewRenderer(termimg.Halfblocks)
	_, err := r.Render("", 20, 16)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if strings.Contains(err.Error(), "20") || strings.Contains(err.Error(), "16") {
		t.Errorf("error should not contain size, got %q", err.Error())
	}
	if msg := err.Error(); msg != "thumbnail: empty path" {
		t.Errorf("unexpected message %q", msg)
	}
}

func TestRenderMissingFileSanitized(t *testing.T) {
	r := NewRenderer(termimg.Halfblocks)
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.png")
	_, err := r.Render(path, 20, 16)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	msg := err.Error()
	if strings.Contains(msg, "nope.png") || strings.Contains(msg, dir) {
		t.Errorf("error leaks path, got %q", msg)
	}
}

func TestRenderInvalidImageSanitized(t *testing.T) {
	r := NewRenderer(termimg.Halfblocks)
	dir := t.TempDir()
	fname := "bad.png"
	path := filepath.Join(dir, fname)
	if err := os.WriteFile(path, []byte("this is not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := r.Render(path, 20, 16)
	if err == nil {
		t.Fatal("expected error for invalid image")
	}
	msg := err.Error()
	if strings.Contains(msg, fname) || strings.Contains(msg, dir) {
		t.Errorf("error leaks path, got %q", msg)
	}
}
