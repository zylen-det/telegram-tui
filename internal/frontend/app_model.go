package frontend

import (
	"errors"
	"image"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// AppModel adapts Bubble Tea messages to the application's event loop.
type AppModel struct {
	engine               *app.Engine
	runtime              *AppRuntime
	location             *time.Location
	surface              *surfaceState
	overlay              *OutputOverlay
	metadata             *appModelMetadata
	composerText         *composerTextHost
	authorizationInput   *authorizationInputHost
	photoPathInput       *photoPathInputHost
	messageSearchInput   *messageSearchInputHost
	chatSearchInput      *chatSearchInputHost
	chatTitleInput       *chatSettingsInputHost
	chatDescriptionInput *chatSettingsInputHost
	// listModals owns the single persistent selector host plus the one
	// descriptor/option synchronization path shared by every list modal.
	listModals *listModalController
}

type appModelMetadata struct {
	mu          sync.RWMutex
	shutdownErr error
}

// SetOutputOverlay connects View's modal state to serialized terminal output.
func (m *AppModel) SetOutputOverlay(overlay *OutputOverlay) {
	if m == nil {
		return
	}
	m.overlay = overlay
}

type appEventMsg struct {
	event app.Event
}

type appRuntimeStoppedMsg struct{}

type toastExpiredMsg struct{ generation uint64 }

// ProcessQuitMsg asks AppModel to begin the application's graceful shutdown.
// Production signal handling sends this message instead of quitting Bubble Tea.
type ProcessQuitMsg struct{}

// NewAppModel creates an app-backed Bubble Tea authorization model.
func NewAppModel(engine *app.Engine, runtime *AppRuntime) (AppModel, error) {
	if engine == nil {
		return AppModel{}, errors.New("frontend: nil app engine")
	}
	if runtime == nil {
		return AppModel{}, errors.New("frontend: nil app runtime")
	}
	return AppModel{
		engine:               engine,
		runtime:              runtime,
		location:             time.Local,
		surface:              &surfaceState{},
		metadata:             &appModelMetadata{},
		composerText:         newComposerTextHost(),
		authorizationInput:   newAuthorizationInputHost(),
		photoPathInput:       newPhotoPathInputHost(),
		messageSearchInput:   newMessageSearchInputHost(),
		chatSearchInput:      newChatSearchInputHost(),
		chatTitleInput:       newChatSettingsInputHost(),
		chatDescriptionInput: newChatSettingsInputHost(),
		listModals:           newListModalController(),
	}, nil
}

func (m AppModel) Init() tea.Cmd {
	return m.commandsAndWait(m.engine.Apply(app.Started{}))
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProcessQuitMsg:
		return m, m.commandsAndWait(m.engine.Apply(app.ActionReceived{Action: app.Quit}))
	case tea.FocusMsg:
		return m, m.deliver(m.engine.Apply(app.TerminalFocusChanged{Focused: true}))
	case tea.BlurMsg:
		return m, m.deliver(m.engine.Apply(app.TerminalFocusChanged{Focused: false}))
	case tea.WindowSizeMsg:
		return m, tea.Batch(m.deliver(m.engine.Apply(app.Resized{
			Width:  max(0, msg.Width),
			Height: max(0, msg.Height),
		})), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
	case tea.PasteMsg:
		if focus := m.engine.Snapshot().Focus; focus == app.FocusComposer {
			var preSync tea.Cmd
			if snapForID := m.engine.Snapshot(); snapForID.SelectedChat >= 0 && snapForID.SelectedChat < len(snapForID.Chats) {
				wantID := snapForID.Chats[snapForID.SelectedChat].ID
				var wantEdit domain.MessageID
				if snapForID.EditTarget != nil && snapForID.EditTarget.ChatID == wantID {
					wantEdit = snapForID.EditTarget.MessageID
				}
				wantFocused := snapForID.Focus == app.FocusComposer
				if m.composerText.Identity() != (composerTextIdentity{ChatID: wantID, EditMessageID: wantEdit}) || m.composerText.focused != wantFocused {
					preSync = m.syncComposerTextHost()
				}
			} else if m.composerText.Identity() != (composerTextIdentity{}) || m.composerText.focused {
				preSync = m.syncComposerTextHost()
			}
			changed, value, cmd := m.composerText.Update(msg)
			var commands []app.Command
			if changed {
				commands = m.engine.Apply(app.ComposerValueChanged{
					ChatID:        m.composerText.Identity().ChatID,
					EditMessageID: m.composerText.Identity().EditMessageID,
					Value:         value,
				})
			}
			return m, tea.Batch(preSync, m.deliver(commands), cmd)
		}
		if focus := m.engine.Snapshot().Focus; focus == app.FocusAuth {
			preSync := m.syncAuthorizationInputHost()
			changed, value, cmd := m.authorizationInput.Update(msg)
			if changed {
				commands := m.engine.Apply(app.PromptValueChanged{
					PromptID: m.authorizationInput.Identity(),
					Value:    value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus := m.engine.Snapshot().Focus; focus == app.FocusPhotoSend {
			preSync := m.syncPhotoPathInputHost()
			paste := msg
			paste.Content = strings.Map(func(r rune) rune {
				if r == '\r' || r == '\n' {
					return -1
				}
				return r
			}, msg.Content)
			changed, value, cmd := m.photoPathInput.Update(paste)
			if changed {
				commands := m.engine.Apply(app.PhotoPathValueChanged{
					ChatID: m.photoPathInput.Identity(),
					Value:  value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus := m.engine.Snapshot().Focus; focus == app.FocusSearchInput {
			preSync := m.syncMessageSearchInputHost()
			paste := msg
			paste.Content = strings.Map(func(r rune) rune {
				if r == '\r' || r == '\n' {
					return -1
				}
				return r
			}, msg.Content)
			changed, value, cmd := m.messageSearchInput.Update(paste)
			if changed {
				commands := m.engine.Apply(app.MessageSearchValueChanged{ChatID: m.messageSearchInput.Identity(), Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
		}
		if focus := m.engine.Snapshot().Focus; focus == app.FocusChatSearchInput {
			preSync := m.syncChatSearchInputHost()
			paste := msg
			paste.Content = strings.Map(func(r rune) rune {
				if r == '\r' || r == '\n' {
					return -1
				}
				return r
			}, msg.Content)
			changed, value, cmd := m.chatSearchInput.Update(paste)
			if changed {
				commands := m.engine.Apply(app.ChatSearchValueChanged{Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSearchInputHost(), m.syncListModalController())
		}
		if focus := m.engine.Snapshot().Focus; focus == app.FocusChatSettingsInput {
			preSync := m.syncChatSettingsInputHosts()
			snap := m.engine.Snapshot()
			host, field, editorID := m.chatSettingsEditor(snap.ChatSettings)
			if host == nil {
				return m, preSync
			}
			changed, value, cmd := host.Update(msg)
			if changed {
				if snap.ChatSettings != nil {
					commands := m.engine.Apply(app.ChatSettingsValueChanged{ChatID: snap.ChatSettings.ChatID, Field: field, EditorID: editorID, Value: value})
					return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
				}
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case tea.KeyPressMsg:
		snapshot := m.engine.Snapshot()
		focus := snapshot.Focus
		if received, ok := mapCommandMenuKey(snapshot.CommandMenu != nil, msg); ok {
			commands := m.engine.Apply(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost())
		}
		if received, ok := mapKeyPress(focus, msg); ok {
			if snap := m.engine.Snapshot(); snap.StickerPicker != nil {
				switch received.Action {
				case app.StickerMoveLeft, app.StickerMoveRight, app.StickerMoveUp, app.StickerMoveDown, app.StickerActivate, app.Close:
					received.RequestID = snap.StickerPicker.RequestID
				}
			}
			switch received.Action {
			case app.ComposerBackspace, app.ComposerNewline:
				if focus == app.FocusAuth {
					preSync := m.syncAuthorizationInputHost()
					changed, value, cmd := m.authorizationInput.Update(msg)
					if changed {
						commands := m.engine.Apply(app.PromptValueChanged{
							PromptID: m.authorizationInput.Identity(),
							Value:    value,
						})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
				}
				if focus == app.FocusPhotoSend {
					preSync := m.syncPhotoPathInputHost()
					changed, value, cmd := m.photoPathInput.Update(msg)
					if changed {
						commands := m.engine.Apply(app.PhotoPathValueChanged{
							ChatID: m.photoPathInput.Identity(),
							Value:  value,
						})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
				}
				if focus == app.FocusSearchInput {
					preSync := m.syncMessageSearchInputHost()
					changed, value, cmd := m.messageSearchInput.Update(msg)
					if changed {
						commands := m.engine.Apply(app.MessageSearchValueChanged{ChatID: m.messageSearchInput.Identity(), Value: value})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
				}
				if focus == app.FocusChatSearchInput {
					preSync := m.syncChatSearchInputHost()
					changed, value, cmd := m.chatSearchInput.Update(msg)
					if changed {
						commands := m.engine.Apply(app.ChatSearchValueChanged{Value: value})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSearchInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncChatSearchInputHost(), m.syncListModalController())
				}
				if focus == app.FocusChatSettingsInput {
					preSync := m.syncChatSettingsInputHosts()
					snap := m.engine.Snapshot()
					host, field, editorID := m.chatSettingsEditor(snap.ChatSettings)
					if host == nil {
						return m, preSync
					}
					changed, value, cmd := host.Update(msg)
					if changed {
						if snap.ChatSettings != nil {
							commands := m.engine.Apply(app.ChatSettingsValueChanged{ChatID: snap.ChatSettings.ChatID, Field: field, EditorID: editorID, Value: value})
							return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
						}
					}
					return m, tea.Batch(preSync, cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
				}
				if focus != app.FocusComposer {
					return m, m.deliver(m.engine.Apply(received))
				}
				if focus != app.FocusComposer {
					return m, m.deliver(m.engine.Apply(received))
				}
				// Ensure host identity/focus matches authoritative snapshot before
				// handling input (first key after startup/chat switch/focus change).
				// This check is cheap; the full Select only runs when identity
				// or focus actually differs, so long-press repeats stay lightweight
				// like crush's textarea.
				var preSync tea.Cmd
				if snapForID := m.engine.Snapshot(); snapForID.SelectedChat >= 0 && snapForID.SelectedChat < len(snapForID.Chats) {
					wantID := snapForID.Chats[snapForID.SelectedChat].ID
					var wantEdit domain.MessageID
					if snapForID.EditTarget != nil && snapForID.EditTarget.ChatID == wantID {
						wantEdit = snapForID.EditTarget.MessageID
					}
					wantFocused := snapForID.Focus == app.FocusComposer
					if m.composerText.Identity() != (composerTextIdentity{ChatID: wantID, EditMessageID: wantEdit}) || m.composerText.focused != wantFocused {
						preSync = m.syncComposerTextHost()
					}
				} else if m.composerText.Identity() != (composerTextIdentity{}) || m.composerText.focused {
					preSync = m.syncComposerTextHost()
				}
				changed, value, cmd := m.composerText.Update(msg)
				var commands []app.Command
				if changed {
					// Keep authoritative Draft/Edit in sync for persistence,
					// but avoid heavy Select/Sync per keystroke (crush-like).
					commands = m.engine.Apply(app.ComposerValueChanged{
						ChatID:        m.composerText.Identity().ChatID,
						EditMessageID: m.composerText.Identity().EditMessageID,
						Value:         value,
					})
				}
				return m, tea.Batch(preSync, m.deliver(commands), cmd)
			case app.ComposerSubmit:
				return m, tea.Batch(m.deliver(m.engine.Apply(received)), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
			default:
				commands := m.engine.Apply(received)
				return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncListModalController())
			}
		}
		if focus == app.FocusComposer && textInputAllowed(msg.Key()) {
			var preSync tea.Cmd
			if snapForID := m.engine.Snapshot(); snapForID.SelectedChat >= 0 && snapForID.SelectedChat < len(snapForID.Chats) {
				wantID := snapForID.Chats[snapForID.SelectedChat].ID
				var wantEdit domain.MessageID
				if snapForID.EditTarget != nil && snapForID.EditTarget.ChatID == wantID {
					wantEdit = snapForID.EditTarget.MessageID
				}
				wantFocused := snapForID.Focus == app.FocusComposer
				if m.composerText.Identity() != (composerTextIdentity{ChatID: wantID, EditMessageID: wantEdit}) || m.composerText.focused != wantFocused {
					preSync = m.syncComposerTextHost()
				}
			} else if m.composerText.Identity() != (composerTextIdentity{}) || m.composerText.focused {
				preSync = m.syncComposerTextHost()
			}
			changed, value, cmd := m.composerText.Update(msg)
			var commands []app.Command
			if changed {
				commands = m.engine.Apply(app.ComposerValueChanged{
					ChatID:        m.composerText.Identity().ChatID,
					EditMessageID: m.composerText.Identity().EditMessageID,
					Value:         value,
				})
			}
			return m, tea.Batch(preSync, m.deliver(commands), cmd)
		}
		if focus == app.FocusAuth && textInputAllowed(msg.Key()) {
			preSync := m.syncAuthorizationInputHost()
			changed, value, cmd := m.authorizationInput.Update(msg)
			if changed {
				commands := m.engine.Apply(app.PromptValueChanged{
					PromptID: m.authorizationInput.Identity(),
					Value:    value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus == app.FocusPhotoSend && photoEditKeyAllowed(msg.Key()) {
			preSync := m.syncPhotoPathInputHost()
			changed, value, cmd := m.photoPathInput.Update(msg)
			if changed {
				commands := m.engine.Apply(app.PhotoPathValueChanged{
					ChatID: m.photoPathInput.Identity(),
					Value:  value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus == app.FocusSearchInput && textInputAllowed(msg.Key()) {
			preSync := m.syncMessageSearchInputHost()
			changed, value, cmd := m.messageSearchInput.Update(msg)
			if changed {
				commands := m.engine.Apply(app.MessageSearchValueChanged{ChatID: m.messageSearchInput.Identity(), Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
		}
		if focus == app.FocusChatSearchInput && textInputAllowed(msg.Key()) {
			preSync := m.syncChatSearchInputHost()
			changed, value, cmd := m.chatSearchInput.Update(msg)
			if changed {
				commands := m.engine.Apply(app.ChatSearchValueChanged{Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSearchInputHost(), m.syncListModalController())
		}
		if focus == app.FocusChatSettingsInput && textInputAllowed(msg.Key()) {
			preSync := m.syncChatSettingsInputHosts()
			snap := m.engine.Snapshot()
			host, field, editorID := m.chatSettingsEditor(snap.ChatSettings)
			if host == nil {
				return m, preSync
			}
			changed, value, cmd := host.Update(msg)
			if changed {
				if snap.ChatSettings != nil {
					commands := m.engine.Apply(app.ChatSettingsValueChanged{ChatID: snap.ChatSettings.ChatID, Field: field, EditorID: editorID, Value: value})
					return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
				}
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case tea.MouseClickMsg:
		if received, ok := mapMouseClick(msg, m.hitRegions()); ok {
			commands := m.engine.Apply(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case tea.MouseWheelMsg:
		if received, ok := mapMouseWheel(msg, m.hitRegions()); ok {
			commands := m.engine.Apply(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case toastExpiredMsg:
		commands := m.engine.Apply(app.ToastExpired{Generation: msg.generation})
		return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
	case appEventMsg:
		commands := m.engine.Apply(msg.event)
		if complete, ok := msg.event.(app.ShutdownComplete); ok {
			m.recordShutdownError(complete.Error)
			return m, m.shutdownAndQuit()
		}
		return m, tea.Batch(m.commandsAndWait(commands), m.toastExpiryCommand(), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
	case appRuntimeStoppedMsg:
		return m, tea.Quit
	default:
		return m, nil
	}
}

// ShutdownError returns the fatal error reported by ShutdownComplete, if any.
func (m AppModel) ShutdownError() error {
	if m.metadata == nil {
		return nil
	}
	m.metadata.mu.RLock()
	defer m.metadata.mu.RUnlock()
	if m.metadata.shutdownErr == nil {
		return nil
	}
	if err, ok := m.metadata.shutdownErr.(*domain.AppError); ok && err == nil {
		return nil
	}
	return m.metadata.shutdownErr
}

func (m AppModel) recordShutdownError(shutdownErr error) {
	if m.metadata == nil {
		return
	}
	if shutdownErr != nil {
		if err, ok := shutdownErr.(*domain.AppError); ok && err == nil {
			shutdownErr = nil
		}
	}
	m.metadata.mu.Lock()
	m.metadata.shutdownErr = shutdownErr
	m.metadata.mu.Unlock()
}

func photoEditKeyAllowed(key tea.Key) bool {
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	return key.Mod&^tea.ModShift == 0
}

func (m AppModel) commandsAndWait(commands []app.Command) tea.Cmd {
	return tea.Batch(m.deliver(commands), m.runtime.waitEvent())
}

func (m AppModel) deliver(commands []app.Command) tea.Cmd {
	return m.runtime.deliver(commands)
}

func (m AppModel) toastExpiryCommand() tea.Cmd {
	state := m.engine.Snapshot()
	if state.Toast == nil || state.ToastDuration <= 0 || state.ToastGeneration == 0 {
		return nil
	}
	generation, duration := state.ToastGeneration, state.ToastDuration
	return tea.Tick(duration, func(time.Time) tea.Msg { return toastExpiredMsg{generation: generation} })
}

func (m AppModel) syncComposerTextHost() tea.Cmd {
	snap := m.engine.Snapshot()

	// Determine active chat identity.
	var (
		chatID        domain.ChatID
		editMessageID domain.MessageID
	)
	valid := snap.SelectedChat >= 0 && snap.SelectedChat < len(snap.Chats)
	if valid {
		chatID = snap.Chats[snap.SelectedChat].ID
	}

	// Same-chat edit target scopes the identity (the draft value itself is
	// already topic/edit-routed by the view model).
	if valid && snap.EditTarget != nil && snap.EditTarget.ChatID == chatID {
		editMessageID = snap.EditTarget.MessageID
	}

	// Focused iff authoritative focus is composer.
	focused := snap.Focus == app.FocusComposer

	model := ui.Select(snap, m.location)
	composerRect := composerSurfaceRect(model)
	textRect := composerTextRect(model, composerRect)

	width := textRect.Dx()
	height := textRect.Dy()

	// TopicID scopes the host so a topic switch resets the embedded editor.
	var topicID domain.TopicID
	if valid {
		topicID = snap.SelectedTopics[chatID]
	}

	return m.composerText.Sync(
		composerTextIdentity{
			ChatID:        chatID,
			EditMessageID: editMessageID,
			TopicID:       topicID,
		},
		model.Draft,
		focused,
		width,
		height,
	)
}

func (m AppModel) syncAuthorizationInputHost() tea.Cmd {
	snap := m.engine.Snapshot()

	var (
		promptID uint64
		value    string
		secret   bool
	)
	if snap.Prompt != nil {
		promptID = snap.Prompt.Prompt.ID
		value = string(snap.Prompt.Input)
		secret = snap.Prompt.Prompt.Secret
	}

	focused := snap.Prompt != nil && snap.Focus == app.FocusAuth
	width := authorizationInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()

	return m.authorizationInput.Sync(promptID, value, focused, secret, width)
}

func (m *AppModel) syncPhotoPathInputHost() tea.Cmd {
	snap := m.engine.Snapshot()

	var (
		chatID domain.ChatID
		value  string
	)
	if snap.PhotoSend != nil {
		chatID = snap.PhotoSend.ChatID
		value = string(snap.PhotoSend.Input)
	}

	focused := snap.PhotoSend != nil && snap.Focus == app.FocusPhotoSend
	width := photoSendInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()

	return m.photoPathInput.Sync(chatID, value, focused, width)
}

func (m *AppModel) syncMessageSearchInputHost() tea.Cmd {
	snap := m.engine.Snapshot()
	var chatID domain.ChatID
	var value string
	if snap.MessageSearch != nil {
		chatID = snap.MessageSearch.ChatID
		value = string(snap.MessageSearch.Input)
	}
	focused := snap.MessageSearch != nil && snap.Focus == app.FocusSearchInput
	width := messageSearchInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()
	return m.messageSearchInput.Sync(chatID, value, focused, width)
}

func (m *AppModel) syncChatSearchInputHost() tea.Cmd {
	snap := m.engine.Snapshot()
	var value string
	if snap.ChatSearch != nil {
		value = string(snap.ChatSearch.Input)
	}
	focused := snap.ChatSearch != nil && snap.Focus == app.FocusChatSearchInput
	width := chatSearchInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()
	return m.chatSearchInput.Sync(value, focused, width)
}

func (m *AppModel) chatSettingsEditor(settings *app.ChatSettingsState) (*chatSettingsInputHost, app.ChatSettingField, uint64) {
	if settings == nil {
		return nil, 0, 0
	}
	switch settings.Mode {
	case app.ChatSettingsTitleEditor:
		return m.chatTitleInput, app.ChatSettingTitle, settings.TitleEditorID
	case app.ChatSettingsDescriptionEditor:
		return m.chatDescriptionInput, app.ChatSettingDescription, settings.DescriptionEditorID
	default:
		return nil, 0, 0
	}
}

func (m *AppModel) syncChatSettingsInputHosts() tea.Cmd {
	snap := m.engine.Snapshot()
	var titleID, descriptionID uint64
	var title, description string
	var titleFocused, descriptionFocused bool
	if snap.ChatSettings != nil {
		titleID = snap.ChatSettings.TitleEditorID
		descriptionID = snap.ChatSettings.DescriptionEditorID
		title = string(snap.ChatSettings.TitleInput)
		description = string(snap.ChatSettings.DescriptionInput)
		titleFocused = snap.Focus == app.FocusChatSettingsInput && snap.ChatSettings.Mode == app.ChatSettingsTitleEditor
		descriptionFocused = snap.Focus == app.FocusChatSettingsInput && snap.ChatSettings.Mode == app.ChatSettingsDescriptionEditor
	}
	width := max(1, snap.Width/2)
	return tea.Batch(
		m.chatTitleInput.Sync(titleID, title, titleFocused, width),
		m.chatDescriptionInput.Sync(descriptionID, description, descriptionFocused, width),
	)
}

// syncListModalController projects one snapshot into the shared list modal
// controller. The controller resolves the single active listModalDescriptor
// (identity, exact displayed rows, derived options, authoritative semantic
// selection, focus, preferred modal width, and the bounded PreferEdit
// override) and synchronizes its one selector host, so option order and
// geometry can never disagree with the rendered frame.
//
// The reducer-driven selection model stays untouched: the host is a render and
// geometry mirror of authoritative state, never an input owner. When no list
// modal is active this skips the heavy ui.Select projection (clones
// chats/avatars/groups) that composer typing would otherwise run at 30 Hz.
func (m AppModel) syncListModalController() tea.Cmd {
	snap := m.engine.Snapshot()
	bounds := image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))
	if !listModalSnapshotActive(snap) {
		return m.listModals.Reset(bounds)
	}
	return m.listModals.Sync(ui.Select(snap, m.location), m.location)
}

func (m AppModel) shutdownAndQuit() tea.Cmd {
	return func() tea.Msg {
		m.runtime.Close()
		m.runtime.Wait()
		return tea.QuitMsg{}
	}
}
