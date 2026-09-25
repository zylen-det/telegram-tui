package frontend

import (
	"errors"
	"image"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// AppModel is the Bubble Tea application model and owns the current State.
type AppModel struct {
	state                *State
	session              *Handler
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

type toastExpiredMsg struct{ generation uint64 }

// ProcessQuitMsg asks AppModel to begin the application's graceful shutdown.
// Production signal handling sends this message instead of quitting Bubble Tea.
type ProcessQuitMsg struct{}

// NewAppModel creates an app-backed Bubble Tea model that takes ownership of
// initial. The supplied value is copied into a distinct State, but AppModel
// keeps the caller's nested maps, slices, and pointer members: do not read or
// mutate them through the caller's copy afterwards. Snapshot returns a shallow,
// synchronous read-only view of the owned state; it is not a deep snapshot and
// is not safe to hold across goroutines while Update is running.
func NewAppModel(initial State, session *Handler) (AppModel, error) {
	if session == nil {
		return AppModel{}, errors.New("frontend: nil effect session")
	}
	state := initial
	return AppModel{
		state:                &state,
		session:              session,
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

// applyMessage reduces one event in place against the AppModel-owned state.
// Init starts the loop; subsequent mutations run through Update. Effect commands
// run off-loop and only return messages for Update to apply.
func (m AppModel) applyMessage(event Event) []Effect {
	return updateState(m.state, event)
}

// Snapshot returns the current application state value. It is a shallow,
// synchronous read-only view, not a deep snapshot, and is not safe to carry
// across goroutines while the Update loop is running. Callers that read more
// than one field take a single snapshot and reuse it.
func (m AppModel) Snapshot() State {
	if m.state == nil {
		return InitialState()
	}
	return *m.state
}

func (m AppModel) Init() tea.Cmd {
	// Starting the client is one command; the Telegram update and authorization
	// prompt streams each have exactly one outstanding subscription command,
	// re-armed by Update as each stream item is handled.
	return tea.Batch(
		m.deliver(m.applyMessage(Started{})),
		waitForUpdate(m.session.Updates()),
		waitForPrompt(m.session.PromptStream()),
	)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProcessQuitMsg:
		return m, m.deliver(m.applyMessage(ActionReceived{Action: Quit}))
	case tea.FocusMsg:
		return m, m.deliver(m.applyMessage(TerminalFocusChanged{Focused: true}))
	case tea.BlurMsg:
		return m, m.deliver(m.applyMessage(TerminalFocusChanged{Focused: false}))
	case tea.WindowSizeMsg:
		return m, tea.Batch(m.deliver(m.applyMessage(Resized{
			Width:  max(0, msg.Width),
			Height: max(0, msg.Height),
		})), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
	case tea.PasteMsg:
		if focus := m.Snapshot().Focus; focus == FocusComposer {
			var preSync tea.Cmd
			if snapForID := m.Snapshot(); snapForID.SelectedChat >= 0 && snapForID.SelectedChat < len(snapForID.Chats) {
				wantID := snapForID.Chats[snapForID.SelectedChat].ID
				var wantEdit domain.MessageID
				if snapForID.EditTarget != nil && snapForID.EditTarget.ChatID == wantID {
					wantEdit = snapForID.EditTarget.MessageID
				}
				wantFocused := snapForID.Focus == FocusComposer
				if m.composerText.Identity() != (composerTextIdentity{ChatID: wantID, EditMessageID: wantEdit}) || m.composerText.focused != wantFocused {
					preSync = m.syncComposerTextHost()
				}
			} else if m.composerText.Identity() != (composerTextIdentity{}) || m.composerText.focused {
				preSync = m.syncComposerTextHost()
			}
			changed, value, cmd := m.composerText.Update(msg)
			var commands []Effect
			if changed {
				commands = m.applyMessage(ComposerValueChanged{
					ChatID:        m.composerText.Identity().ChatID,
					EditMessageID: m.composerText.Identity().EditMessageID,
					Value:         value,
				})
			}
			return m, tea.Batch(preSync, m.deliver(commands), cmd)
		}
		if focus := m.Snapshot().Focus; focus == FocusAuth {
			preSync := m.syncAuthorizationInputHost()
			changed, value, cmd := m.authorizationInput.Update(msg)
			if changed {
				commands := m.applyMessage(PromptValueChanged{
					PromptID: m.authorizationInput.Identity(),
					Value:    value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus := m.Snapshot().Focus; focus == FocusPhotoSend {
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
				commands := m.applyMessage(PhotoPathValueChanged{
					ChatID: m.photoPathInput.Identity(),
					Value:  value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus := m.Snapshot().Focus; focus == FocusSearchInput {
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
				commands := m.applyMessage(MessageSearchValueChanged{ChatID: m.messageSearchInput.Identity(), Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
		}
		if focus := m.Snapshot().Focus; focus == FocusChatSearchInput {
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
				commands := m.applyMessage(ChatSearchValueChanged{Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSearchInputHost(), m.syncListModalController())
		}
		if focus := m.Snapshot().Focus; focus == FocusChatSettingsInput {
			preSync := m.syncChatSettingsInputHosts()
			snap := m.Snapshot()
			host, field, editorID := m.chatSettingsEditor(snap.ChatSettings)
			if host == nil {
				return m, preSync
			}
			changed, value, cmd := host.Update(msg)
			if changed {
				if snap.ChatSettings != nil {
					commands := m.applyMessage(ChatSettingsValueChanged{ChatID: snap.ChatSettings.ChatID, Field: field, EditorID: editorID, Value: value})
					return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
				}
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case tea.KeyPressMsg:
		snapshot := m.Snapshot()
		focus := snapshot.Focus
		if received, consumed := mapActionModalKey(snapshot, msg); consumed {
			if received.Action == NoAction {
				return m, nil
			}
			commands := m.applyMessage(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		if received, ok := mapCommandMenuKey(snapshot.CommandMenu != nil, msg); ok {
			commands := m.applyMessage(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost())
		}
		if received, ok := mapKeyPress(focus, msg); ok {
			if focus == FocusMembers && snapshot.Members != nil && snapshot.Members.Detail == nil &&
				(received.Action == SelectNext || received.Action == SelectPrevious) {
				selected, cmd := m.listModals.NavigateMembers(snapshot, m.location, msg, received.Action)
				if selected.Action == NoAction {
					return m, cmd
				}
				return m, tea.Batch(cmd, m.deliver(m.applyMessage(selected)), m.syncListModalController())
			}
			if snap := m.Snapshot(); snap.StickerPicker != nil {
				switch received.Action {
				case StickerMoveLeft, StickerMoveRight, StickerMoveUp, StickerMoveDown, StickerActivate, Close:
					received.RequestID = snap.StickerPicker.RequestID
				}
			}
			switch received.Action {
			case ComposerBackspace, ComposerNewline:
				if focus == FocusAuth {
					preSync := m.syncAuthorizationInputHost()
					changed, value, cmd := m.authorizationInput.Update(msg)
					if changed {
						commands := m.applyMessage(PromptValueChanged{
							PromptID: m.authorizationInput.Identity(),
							Value:    value,
						})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
				}
				if focus == FocusPhotoSend {
					preSync := m.syncPhotoPathInputHost()
					changed, value, cmd := m.photoPathInput.Update(msg)
					if changed {
						commands := m.applyMessage(PhotoPathValueChanged{
							ChatID: m.photoPathInput.Identity(),
							Value:  value,
						})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
				}
				if focus == FocusSearchInput {
					preSync := m.syncMessageSearchInputHost()
					changed, value, cmd := m.messageSearchInput.Update(msg)
					if changed {
						commands := m.applyMessage(MessageSearchValueChanged{ChatID: m.messageSearchInput.Identity(), Value: value})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
				}
				if focus == FocusChatSearchInput {
					preSync := m.syncChatSearchInputHost()
					changed, value, cmd := m.chatSearchInput.Update(msg)
					if changed {
						commands := m.applyMessage(ChatSearchValueChanged{Value: value})
						return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSearchInputHost(), m.syncListModalController())
					}
					return m, tea.Batch(preSync, cmd, m.syncChatSearchInputHost(), m.syncListModalController())
				}
				if focus == FocusChatSettingsInput {
					preSync := m.syncChatSettingsInputHosts()
					snap := m.Snapshot()
					host, field, editorID := m.chatSettingsEditor(snap.ChatSettings)
					if host == nil {
						return m, preSync
					}
					changed, value, cmd := host.Update(msg)
					if changed {
						if snap.ChatSettings != nil {
							commands := m.applyMessage(ChatSettingsValueChanged{ChatID: snap.ChatSettings.ChatID, Field: field, EditorID: editorID, Value: value})
							return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
						}
					}
					return m, tea.Batch(preSync, cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
				}
				if focus != FocusComposer {
					return m, m.deliver(m.applyMessage(received))
				}
				if focus != FocusComposer {
					return m, m.deliver(m.applyMessage(received))
				}
				// Ensure host identity/focus matches authoritative snapshot before
				// handling input (first key after startup/chat switch/focus change).
				// This check is cheap; the full Select only runs when identity
				// or focus actually differs, so long-press repeats stay lightweight
				// like crush's textarea.
				var preSync tea.Cmd
				if snapForID := m.Snapshot(); snapForID.SelectedChat >= 0 && snapForID.SelectedChat < len(snapForID.Chats) {
					wantID := snapForID.Chats[snapForID.SelectedChat].ID
					var wantEdit domain.MessageID
					if snapForID.EditTarget != nil && snapForID.EditTarget.ChatID == wantID {
						wantEdit = snapForID.EditTarget.MessageID
					}
					wantFocused := snapForID.Focus == FocusComposer
					if m.composerText.Identity() != (composerTextIdentity{ChatID: wantID, EditMessageID: wantEdit}) || m.composerText.focused != wantFocused {
						preSync = m.syncComposerTextHost()
					}
				} else if m.composerText.Identity() != (composerTextIdentity{}) || m.composerText.focused {
					preSync = m.syncComposerTextHost()
				}
				changed, value, cmd := m.composerText.Update(msg)
				var commands []Effect
				if changed {
					// Keep authoritative Draft/Edit in sync for persistence,
					// but avoid heavy Select/Sync per keystroke (crush-like).
					commands = m.applyMessage(ComposerValueChanged{
						ChatID:        m.composerText.Identity().ChatID,
						EditMessageID: m.composerText.Identity().EditMessageID,
						Value:         value,
					})
				}
				return m, tea.Batch(preSync, m.deliver(commands), cmd)
			case ComposerSubmit:
				return m, tea.Batch(m.deliver(m.applyMessage(received)), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
			default:
				commands := m.applyMessage(received)
				return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncListModalController())
			}
		}
		if focus == FocusComposer && textInputAllowed(msg.Key()) {
			var preSync tea.Cmd
			if snapForID := m.Snapshot(); snapForID.SelectedChat >= 0 && snapForID.SelectedChat < len(snapForID.Chats) {
				wantID := snapForID.Chats[snapForID.SelectedChat].ID
				var wantEdit domain.MessageID
				if snapForID.EditTarget != nil && snapForID.EditTarget.ChatID == wantID {
					wantEdit = snapForID.EditTarget.MessageID
				}
				wantFocused := snapForID.Focus == FocusComposer
				if m.composerText.Identity() != (composerTextIdentity{ChatID: wantID, EditMessageID: wantEdit}) || m.composerText.focused != wantFocused {
					preSync = m.syncComposerTextHost()
				}
			} else if m.composerText.Identity() != (composerTextIdentity{}) || m.composerText.focused {
				preSync = m.syncComposerTextHost()
			}
			changed, value, cmd := m.composerText.Update(msg)
			var commands []Effect
			if changed {
				commands = m.applyMessage(ComposerValueChanged{
					ChatID:        m.composerText.Identity().ChatID,
					EditMessageID: m.composerText.Identity().EditMessageID,
					Value:         value,
				})
			}
			return m, tea.Batch(preSync, m.deliver(commands), cmd)
		}
		if focus == FocusAuth && textInputAllowed(msg.Key()) {
			preSync := m.syncAuthorizationInputHost()
			changed, value, cmd := m.authorizationInput.Update(msg)
			if changed {
				commands := m.applyMessage(PromptValueChanged{
					PromptID: m.authorizationInput.Identity(),
					Value:    value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus == FocusPhotoSend && photoEditKeyAllowed(msg.Key()) {
			preSync := m.syncPhotoPathInputHost()
			changed, value, cmd := m.photoPathInput.Update(msg)
			if changed {
				commands := m.applyMessage(PhotoPathValueChanged{
					ChatID: m.photoPathInput.Identity(),
					Value:  value,
				})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncPhotoPathInputHost(), m.syncListModalController())
		}
		if focus == FocusSearchInput && textInputAllowed(msg.Key()) {
			preSync := m.syncMessageSearchInputHost()
			changed, value, cmd := m.messageSearchInput.Update(msg)
			if changed {
				commands := m.applyMessage(MessageSearchValueChanged{ChatID: m.messageSearchInput.Identity(), Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncMessageSearchInputHost(), m.syncListModalController())
		}
		if focus == FocusChatSearchInput && textInputAllowed(msg.Key()) {
			preSync := m.syncChatSearchInputHost()
			changed, value, cmd := m.chatSearchInput.Update(msg)
			if changed {
				commands := m.applyMessage(ChatSearchValueChanged{Value: value})
				return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSearchInputHost(), m.syncListModalController())
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSearchInputHost(), m.syncListModalController())
		}
		if focus == FocusChatSettingsInput && textInputAllowed(msg.Key()) {
			preSync := m.syncChatSettingsInputHosts()
			snap := m.Snapshot()
			host, field, editorID := m.chatSettingsEditor(snap.ChatSettings)
			if host == nil {
				return m, preSync
			}
			changed, value, cmd := host.Update(msg)
			if changed {
				if snap.ChatSettings != nil {
					commands := m.applyMessage(ChatSettingsValueChanged{ChatID: snap.ChatSettings.ChatID, Field: field, EditorID: editorID, Value: value})
					return m, tea.Batch(preSync, m.deliver(commands), cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
				}
			}
			return m, tea.Batch(preSync, cmd, m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case tea.MouseClickMsg:
		if received, ok := mapMouseClick(msg, m.hitRegions()); ok {
			commands := m.applyMessage(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case tea.MouseWheelMsg:
		if received, ok := mapMouseWheel(msg, m.hitRegions()); ok {
			commands := m.applyMessage(received)
			return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
		}
		return m, nil
	case toastExpiredMsg:
		commands := m.applyMessage(ToastExpired{Generation: msg.generation})
		return m, tea.Batch(m.deliver(commands), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
	default:
		commands := m.applyMessage(msg)
		if complete, ok := msg.(ShutdownComplete); ok {
			m.recordShutdownError(complete.Error)
			return m, tea.Quit
		}
		return m, tea.Batch(m.deliver(commands), m.resubscribe(msg), m.toastExpiryCommand(), m.syncComposerTextHost(), m.syncAuthorizationInputHost(), m.syncPhotoPathInputHost(), m.syncMessageSearchInputHost(), m.syncChatSearchInputHost(), m.syncChatSettingsInputHosts(), m.syncListModalController())
	}
}

// resubscribe re-arms the single outstanding subscription command for a stream
// item. Stream closure messages do not re-arm.
func (m AppModel) resubscribe(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case TelegramEvent:
		return waitForUpdate(m.session.Updates())
	case PromptRequested:
		return waitForPrompt(m.session.PromptStream())
	default:
		return nil
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

func (m AppModel) deliver(commands []Effect) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(commands))
	for _, effect := range commands {
		if cmd := m.session.Cmd(effect); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (m AppModel) toastExpiryCommand() tea.Cmd {
	state := m.Snapshot()
	if state.Toast == nil || state.ToastDuration <= 0 || state.ToastGeneration == 0 {
		return nil
	}
	generation, duration := state.ToastGeneration, state.ToastDuration
	return tea.Tick(duration, func(time.Time) tea.Msg { return toastExpiredMsg{generation: generation} })
}

func (m AppModel) syncComposerTextHost() tea.Cmd {
	snap := m.Snapshot()

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
	focused := snap.Focus == FocusComposer

	model := Select(snap, m.location)
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
	snap := m.Snapshot()

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

	focused := snap.Prompt != nil && snap.Focus == FocusAuth
	width := authorizationInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()

	return m.authorizationInput.Sync(promptID, value, focused, secret, width)
}

func (m *AppModel) syncPhotoPathInputHost() tea.Cmd {
	snap := m.Snapshot()

	var (
		chatID domain.ChatID
		value  string
	)
	if snap.PhotoSend != nil {
		chatID = snap.PhotoSend.ChatID
		value = string(snap.PhotoSend.Input)
	}

	focused := snap.PhotoSend != nil && snap.Focus == FocusPhotoSend
	width := photoSendInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()

	return m.photoPathInput.Sync(chatID, value, focused, width)
}

func (m *AppModel) syncMessageSearchInputHost() tea.Cmd {
	snap := m.Snapshot()
	var chatID domain.ChatID
	var value string
	if snap.MessageSearch != nil {
		chatID = snap.MessageSearch.ChatID
		value = string(snap.MessageSearch.Input)
	}
	focused := snap.MessageSearch != nil && snap.Focus == FocusSearchInput
	width := messageSearchInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()
	return m.messageSearchInput.Sync(chatID, value, focused, width)
}

func (m *AppModel) syncChatSearchInputHost() tea.Cmd {
	snap := m.Snapshot()
	var value string
	if snap.ChatSearch != nil {
		value = string(snap.ChatSearch.Input)
	}
	focused := snap.ChatSearch != nil && snap.Focus == FocusChatSearchInput
	width := chatSearchInputRect(image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))).Dx()
	return m.chatSearchInput.Sync(value, focused, width)
}

func (m *AppModel) chatSettingsEditor(settings *ChatSettingsState) (*chatSettingsInputHost, ChatSettingField, uint64) {
	if settings == nil {
		return nil, 0, 0
	}
	switch settings.Mode {
	case ChatSettingsTitleEditor:
		return m.chatTitleInput, ChatSettingTitle, settings.TitleEditorID
	case ChatSettingsDescriptionEditor:
		return m.chatDescriptionInput, ChatSettingDescription, settings.DescriptionEditorID
	default:
		return nil, 0, 0
	}
}

func (m *AppModel) syncChatSettingsInputHosts() tea.Cmd {
	snap := m.Snapshot()
	var titleID, descriptionID uint64
	var title, description string
	var titleFocused, descriptionFocused bool
	if snap.ChatSettings != nil {
		titleID = snap.ChatSettings.TitleEditorID
		descriptionID = snap.ChatSettings.DescriptionEditorID
		title = string(snap.ChatSettings.TitleInput)
		description = string(snap.ChatSettings.DescriptionInput)
		titleFocused = snap.Focus == FocusChatSettingsInput && snap.ChatSettings.Mode == ChatSettingsTitleEditor
		descriptionFocused = snap.Focus == FocusChatSettingsInput && snap.ChatSettings.Mode == ChatSettingsDescriptionEditor
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
// modal is active this skips the heavy Select projection (clones
// chats/avatars/groups) that composer typing would otherwise run at 30 Hz.
func (m AppModel) syncListModalController() tea.Cmd {
	snap := m.Snapshot()
	bounds := image.Rect(0, 0, max(0, snap.Width), max(0, snap.Height))
	if !listModalSnapshotActive(snap) {
		return m.listModals.Reset(bounds)
	}
	return m.listModals.Sync(Select(snap, m.location), m.location)
}
