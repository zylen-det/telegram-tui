package avatar

import (
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
)

func TestRendererRendersRoleDimensionsAndCaches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "avatar.png")
	writePNG(t, path)
	cache := &memoryCache{values: make(map[pixel.Key]pixel.Avatar)}
	renderer := Renderer{Cache: cache, Theme: "dark", Background: color.NRGBA{A: 255}}
	ref := domain.AvatarRef{UniqueID: "small-17"}

	chat, err := renderer.Render(context.Background(), path, ref, RoleChatList)
	if err != nil {
		t.Fatalf("chat Render() error = %v", err)
	}
	if chat.Width != 6 || chat.Height != 3 {
		t.Fatalf("chat dimensions = %dx%d, want 6x3", chat.Width, chat.Height)
	}
	if cache.puts != 1 {
		t.Fatalf("cache puts = %d, want 1", cache.puts)
	}
	if cache.lastPut.UniqueID != "small-17:chat-list" {
		t.Fatalf("persistent cache identity = %q, want role-qualified identity", cache.lastPut.UniqueID)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	cached, err := renderer.Render(context.Background(), path, ref, RoleChatList)
	if err != nil {
		t.Fatalf("cached Render() error = %v", err)
	}
	if !reflect.DeepEqual(cached, chat) || cache.hits != 1 {
		t.Fatalf("cached avatar = %#v, hits = %d", cached, cache.hits)
	}

	groupPath := filepath.Join(t.TempDir(), "group.png")
	writePNG(t, groupPath)
	group, err := renderer.Render(context.Background(), groupPath, ref, RoleMessageGroup)
	if err != nil {
		t.Fatalf("group Render() error = %v", err)
	}
	if group.Width != 4 || group.Height != 2 {
		t.Fatalf("group dimensions = %dx%d, want 4x2", group.Width, group.Height)
	}
	if CacheKey(ref, RoleMessageGroup) != "small-17:message-group" {
		t.Fatalf("CacheKey() = %q", CacheKey(ref, RoleMessageGroup))
	}
	if width, height := Dimensions(RoleChatList); width != 6 || height != 6 {
		t.Fatalf("Dimensions(chat-list) = %dx%d, want 6x6", width, height)
	}
	if width, height := Dimensions(RoleMessageGroup); width != 4 || height != 4 {
		t.Fatalf("Dimensions(message-group) = %dx%d, want 4x4", width, height)
	}
}

func TestRendererHonorsCanceledContextBeforeOpening(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Renderer{}).Render(ctx, "/does/not/exist", domain.AvatarRef{UniqueID: "x"}, RoleChatList)
	if err != context.Canceled {
		t.Fatalf("Render() error = %v, want context.Canceled", err)
	}
}

func TestRendererDecodesGIFJPEGAndPNG(t *testing.T) {
	encoders := map[string]func(io.Writer, image.Image) error{
		"gif": func(writer io.Writer, source image.Image) error { return gif.Encode(writer, source, nil) },
		"jpg": func(writer io.Writer, source image.Image) error { return jpeg.Encode(writer, source, nil) },
		"png": png.Encode,
	}
	for extension, encode := range encoders {
		t.Run(extension, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "avatar."+extension)
			file, err := os.Create(path)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
			if err := encode(file, source); err != nil {
				_ = file.Close()
				t.Fatalf("encode error = %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if _, err := (Renderer{}).Render(context.Background(), path, domain.AvatarRef{UniqueID: extension}, RoleChatList); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
		})
	}
}

type memoryCache struct {
	values  map[pixel.Key]pixel.Avatar
	hits    int
	puts    int
	lastPut pixel.Key
}

func (c *memoryCache) Get(key pixel.Key) (pixel.Avatar, bool, error) {
	value, ok := c.values[key]
	if ok {
		c.hits++
	}
	return value, ok, nil
}

func (c *memoryCache) Put(key pixel.Key, value pixel.Avatar) error {
	c.puts++
	c.lastPut = key
	c.values[key] = value
	return nil
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	image := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			image.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + x*80), G: uint8(30 + y*90), A: 255})
		}
	}
	if err := png.Encode(file, image); err != nil {
		_ = file.Close()
		t.Fatalf("Encode() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
