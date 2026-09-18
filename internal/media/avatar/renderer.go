package avatar

import (
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
)

const rendererVersion = 1

type Role string

const (
	RoleChatList     Role = "chat-list"
	RoleMessageGroup Role = "message-group"
)

type Cache interface {
	Get(pixel.Key) (pixel.Avatar, bool, error)
	Put(pixel.Key, pixel.Avatar) error
}

type Renderer struct {
	Cache      Cache
	Theme      string
	Background color.NRGBA
}

func CacheKey(ref domain.AvatarRef, role Role) string {
	return ref.UniqueID + ":" + string(role)
}

func (r Renderer) Render(ctx context.Context, path string, ref domain.AvatarRef, role Role) (pixel.Avatar, error) {
	if err := ctx.Err(); err != nil {
		return pixel.Avatar{}, err
	}
	width, height := Dimensions(role)
	key := pixel.Key{
		UniqueID:        CacheKey(ref, role),
		PixelWidth:      width,
		PixelHeight:     height,
		Theme:           r.Theme,
		RendererVersion: rendererVersion,
	}
	if r.Cache != nil {
		cached, found, err := r.Cache.Get(key)
		if err != nil {
			return pixel.Avatar{}, fmt.Errorf("read avatar cache: %w", err)
		}
		if found {
			return cached, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return pixel.Avatar{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return pixel.Avatar{}, fmt.Errorf("open avatar image: %w", err)
	}
	defer file.Close()
	source, _, err := image.Decode(file)
	if err != nil {
		return pixel.Avatar{}, fmt.Errorf("decode avatar image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return pixel.Avatar{}, err
	}
	result, err := pixel.Render(source, width, height, r.Background)
	if err != nil {
		return pixel.Avatar{}, fmt.Errorf("render avatar image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return pixel.Avatar{}, err
	}
	if r.Cache != nil {
		if err := r.Cache.Put(key, result); err != nil {
			return pixel.Avatar{}, fmt.Errorf("write avatar cache: %w", err)
		}
	}
	return result, nil
}

func Dimensions(role Role) (int, int) {
	if role == RoleMessageGroup {
		return 4, 4
	}
	return 6, 6
}
