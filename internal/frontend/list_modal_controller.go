package frontend

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// listModalDescriptor is one snapshot of the shared list modal that currently
// owns the persistent Huh selector: its stable identity, the exact displayed
// (already windowed) rows, the selector options and authoritative semantic
// selection derived from those rows, the requested focus, the preferred modal
// frame width, and the bounded PreferEdit authoritative override.
//
// A descriptor is the only bridge between rendering and selector
// synchronization: every field comes from the same row list the renderer
// paints, so option order, geometry, and selection can never disagree with
// the frame.
type listModalDescriptor struct {
	// identity scopes the persistent host: any change resets transient
	// selector state instead of retaining it.
	identity selectorIdentity
	// rows is the exact displayed row list, informational and header rows
	// included, in the order the shared modal paints them.
	rows []modalRowSpec
	// options is the actionable subset of rows in display order.
	options []selectorOption
	// authoritative is the semantic payload of the selected actionable row,
	// or the zero payload when the displayed rows mark none.
	authoritative ActionReceived
	// focused is the requested host focus for this surface.
	focused bool
	// preferredWidth is the modal frame width the shared layout should use.
	// A nonpositive value preserves the compact shared default.
	preferredWidth int
	// preferEdit is the bounded explicit authoritative override for the
	// loading-to-settled message-menu Edit introduction.
	preferEdit bool
}

// newListModalDescriptor is the single descriptor constructor: the option list
// and the authoritative semantic selection are always derived from rows, never
// compiled independently.
func newListModalDescriptor(identity selectorIdentity, rows []modalRowSpec, focused bool, preferredWidth int, preferEdit bool) listModalDescriptor {
	return listModalDescriptor{
		identity:       identity,
		rows:           rows,
		options:        selectorOptionsFromRows(rows),
		authoritative:  selectorRowsAuthoritative(rows),
		focused:        focused,
		preferredWidth: preferredWidth,
		preferEdit:     preferEdit,
	}
}

// rowCount is every displayed row, informational and header rows included, so
// the selector geometry follows the same frame the renderer lays out.
func (d listModalDescriptor) rowCount() int { return len(d.rows) }

// optionCount is the actionable subset of the displayed rows.
func (d listModalDescriptor) optionCount() int { return len(d.options) }

// selectorOptionsFromRows derives the selectable Huh options from displayed
// rows: every row carrying both an ID and a real action, in display order,
// keeping its ID, label, and exact semantic ActionReceived payload.
// Informational and section-header rows stay out of selection.
func selectorOptionsFromRows(rows []modalRowSpec) []selectorOption {
	options := make([]selectorOption, 0, len(rows))
	for _, row := range rows {
		if !rowSelectable(row) {
			continue
		}
		options = append(options, selectorOption{ID: row.ID, Label: row.Label, Value: row.Action})
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

// selectorRowsAuthoritative returns the exact semantic payload of the selected
// actionable row, or the zero payload when no actionable row is selected.
func selectorRowsAuthoritative(rows []modalRowSpec) ActionReceived {
	for _, row := range rows {
		if row.Selected && rowSelectable(row) {
			return row.Action
		}
	}
	return ActionReceived{}
}

// rowSelectable is the one actionable-row predicate shared by rendering and
// synchronization: a row participates in the selector (and in the interactive
// hit list) only when it carries an ID and a real action. Section headers and
// informational rows never do, so they stay out of selection while still
// counting in the modal geometry.
func rowSelectable(row modalRowSpec) bool {
	return !row.Header && row.ID != "" && row.Action.Action != NoAction
}

// listModalController owns the single persistent selector host shared by every
// supported list modal and the one descriptor/option synchronization path.
type listModalController struct {
	host *selectorHost
}

// newListModalController builds the controller around one fresh selector host.
func newListModalController() *listModalController {
	return &listModalController{host: newSelectorHost()}
}

// View returns the live Huh view of the owned host. AppModel.View injects it
// into the composition seam; painting never synchronizes the host.
func (c *listModalController) View() string { return c.host.View() }

// Sync aligns the owned host with the supported list modal active in one
// view-model snapshot, or resets it when no supported modal is active.
func (c *listModalController) Sync(model ViewModel, location *time.Location) tea.Cmd {
	bounds := image.Rect(0, 0, max(0, model.Width), max(0, model.Height))
	descriptor, ok := activeListModalDescriptor(model, location)
	if !ok {
		return c.Reset(bounds)
	}
	rect := selectorHostRectWidth(bounds, descriptor.optionCount(), descriptor.rowCount(), descriptor.preferredWidth)
	// The one explicit authoritative override: the bounded PreferEdit
	// loading-to-settled Edit introduction. Every other transition keeps the
	// host's generic semantic-preservation policy.
	if descriptor.preferEdit &&
		descriptor.authoritative.Action == EditMessage &&
		selectorOptionIndex(c.host.Options(), descriptor.authoritative) < 0 {
		return c.host.SyncAuthoritative(descriptor.identity, descriptor.options, descriptor.authoritative, descriptor.focused, rect.Dx(), rect.Dy())
	}
	return c.host.Sync(descriptor.identity, descriptor.options, descriptor.authoritative, descriptor.focused, rect.Dx(), rect.Dy())
}

// Reset clears the host to the inactive selector state: no identity, no
// options, unfocused, and the compact default geometry.
func (c *listModalController) Reset(bounds image.Rectangle) tea.Cmd {
	rect := selectorHostRect(bounds, 0, 0)
	return c.host.Sync(selectorIdentity{}, nil, ActionReceived{}, false, rect.Dx(), rect.Dy())
}

// listModalSnapshotActive is the cheap pre-projection test for the common
// no-list-modal case. It reads only reducer state, so composer typing never
// runs the heavy Select projection just to discover that no list modal is
// active.
//
// Note: ChatSearch renders its own sectioned manual rows and never consumes
// the shared selector overlay (its headers cannot be represented by a
// headerless Huh option list), so it stays out of this test and out of the
// descriptor switch below.
func listModalSnapshotActive(state State) bool {
	return state.MessageMenu != nil ||
		state.ReactionPicker != nil ||
		state.ForwardPicker != nil ||
		(state.MessageSearch != nil && state.Focus == FocusSearchResults) ||
		(state.PinnedMessages != nil && state.Focus == FocusPinnedResults) ||
		(state.Members != nil && state.Focus == FocusMembers) ||
		(state.Topics != nil && state.Focus == FocusTopics) ||
		(state.ChatActions != nil && state.Focus == FocusChatActions)
}

// activeListModalDescriptor is the single place that decides which shared list
// modal owns the selector, preserving reducer-owned surface priority: chat
// actions, then the message action menu, then the reaction picker, then the
// forward picker, then submitted message-search results, then Members, then
// pinned messages, then topics. It reports false when no supported modal is
// active.
func activeListModalDescriptor(model ViewModel, location *time.Location) (listModalDescriptor, bool) {
	bounds := image.Rect(0, 0, max(0, model.Width), max(0, model.Height))
	switch {
	case model.ChatActions != nil && model.Focus == FocusChatActions:
		menu := model.ChatActions
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorChatActions, RequestID: menu.RequestID, ChatID: menu.ChatID},
			displayedChatActionRows(model),
			true,
			chatActionFrame(bounds).Dx(),
			false,
		), true
	case model.MessageMenu != nil:
		menu := model.MessageMenu
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorMessageActions, RequestID: menu.RequestID, ChatID: menu.ChatID, MessageID: menu.MessageID},
			messageActionRows(menu),
			model.Focus == FocusModal,
			messageActionModalWidth,
			menu.PreferEdit,
		), true
	case model.ReactionPicker != nil:
		picker := model.ReactionPicker
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorReaction, RequestID: picker.RequestID, ChatID: picker.ChatID, MessageID: picker.MessageID},
			reactionRows(picker),
			model.Focus == FocusReactionPicker,
			0,
			false,
		), true
	case model.ForwardPicker != nil:
		picker := model.ForwardPicker
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorForward, RequestID: picker.RequestID, ChatID: picker.SourceChatID, MessageID: picker.SourceMessageID},
			forwardRows(picker, model.Chats),
			model.Focus == FocusForwardPicker,
			0,
			false,
		), true
	case model.MessageSearch != nil && model.Focus == FocusSearchResults:
		search := model.MessageSearch
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorMessageSearch, RequestID: search.RequestID, ChatID: search.ChatID},
			displayedMessageSearchRows(model, location),
			true,
			messageSearchFrame(bounds).Dx(),
			false,
		), true
	case model.Members != nil && model.Focus == FocusMembers:
		members := model.Members
		// The single modal has two modes; the detail user scopes the identity
		// so list/detail transitions reset the host, and the detail chrome
		// (avatar, info headers, Working row) is part of the displayed rows.
		var detailUser domain.UserID
		if members.Detail != nil {
			detailUser = members.Detail.UserID
		}
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorMembers, RequestID: members.RequestID, ChatID: members.ChatID, MessageID: domain.MessageID(detailUser)},
			displayedMembersRows(model, false),
			true,
			membersFrame(bounds).Dx(),
			false,
		), true
	case model.PinnedMessages != nil && model.Focus == FocusPinnedResults:
		pinned := model.PinnedMessages
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorPinnedMessages, RequestID: pinned.RequestID, ChatID: pinned.ChatID},
			displayedPinnedMessagesRows(model, location),
			true,
			pinnedMessagesFrame(bounds).Dx(),
			false,
		), true
	case model.Topics != nil && model.Focus == FocusTopics:
		topics := model.Topics
		return newListModalDescriptor(
			selectorIdentity{Kind: selectorTopics, RequestID: topics.RequestID, ChatID: topics.ChatID},
			displayedTopicsRows(model),
			true,
			topicsFrame(bounds).Dx(),
			false,
		), true
	}
	return listModalDescriptor{}, false
}
