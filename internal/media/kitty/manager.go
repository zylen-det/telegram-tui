package kitty

import (
	"errors"
	"image"
	"io"
	"sync"
)

type Manager struct {
	mu       sync.Mutex
	writer   io.Writer
	nextID   uint32
	activeID uint32
}

func NewManager(writer io.Writer) *Manager {
	return &Manager{
		writer: writer,
		nextID: 1,
	}
}

func (m *Manager) ActiveID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeID
}

func (m *Manager) Show(rectangle image.Rectangle, source image.Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeID != 0 {
		if err := DeleteImage(m.writer, m.activeID); err != nil {
			return err
		}
		m.activeID = 0
	}

	imageID := m.allocateID()
	if err := TransmitPNG(m.writer, imageID, rectangle, source); err != nil {
		cleanupErr := DeleteImage(m.writer, imageID)
		if cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		return err
	}

	m.activeID = imageID
	return nil
}

func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeID == 0 {
		return nil
	}
	if err := DeleteImage(m.writer, m.activeID); err != nil {
		return err
	}
	m.activeID = 0
	return nil
}

func (m *Manager) allocateID() uint32 {
	imageID := m.nextID
	if imageID == 0 {
		imageID = 1
	}
	m.nextID = imageID + 1
	if m.nextID == 0 {
		m.nextID = 1
	}
	return imageID
}
