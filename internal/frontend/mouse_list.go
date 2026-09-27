package frontend

import (
	"fmt"
	"time"
)

// MouseClickMsg has no click count. A second press on the same live row within
// this interval activates it; the first press only moves the keyboard cursor.
const doubleClickInterval = 400 * time.Millisecond

type pendingListClick struct {
	context any
	action  ActionReceived
	id      string
	at      time.Time
}

// mouseListContext distinguishes a reopened modal from the old one even when
// its rows have the same labels and actions. Only the topmost list is used.
func mouseListContext(s *State) any {
	switch {
	case s.CommandMenu != nil:
		return s.CommandMenu
	case s.ChatSearch != nil:
		return s.ChatSearch
	case s.ChatActions != nil:
		return s.ChatActions
	case s.Topics != nil:
		return s.Topics
	case s.Members != nil:
		return s.Members
	case s.Administration != nil:
		return s.Administration
	case s.ChatSettings != nil:
		return s.ChatSettings
	case s.InviteLinks != nil:
		return s.InviteLinks
	case s.PinnedMessages != nil:
		return s.PinnedMessages
	case s.MessageSearch != nil:
		return s.MessageSearch
	case s.StickerPicker != nil:
		return s.StickerPicker
	case s.MessageMenu != nil:
		return s.MessageMenu
	case s.ReactionPicker != nil:
		return s.ReactionPicker
	case s.ForwardPicker != nil:
		return s.ForwardPicker
	case s.Modal != nil || s.PhotoSend != nil:
		return nil
	default:
		// The state pointer owns the chat list and details pane for this loop.
		return s
	}
}

// focusMouseListRow checks the current row source and its identity before
// changing the selection. Stale hits cannot focus or activate a different row.
func focusMouseListRow(s *State, hit Hit) bool {
	a := hit.Click
	match := func(rows []modalRowSpec) int {
		for i, row := range rows {
			if rowSelectable(row) && row.Action == a {
				return i
			}
		}
		return -1
	}
	switch {
	case s.CommandMenu != nil:
		menu := s.CommandMenu
		if a.Action != CommandMenuActivate || a.ChatID != menu.ChatID || a.CommandIndex < 0 || a.CommandIndex >= len(menu.Candidates) || menu.Loading || menu.Error != nil {
			return false
		}
		menu.Selected = a.CommandIndex
		ensureCommandMenuSelectionVisible(menu)
		return true
	case s.ChatSearch != nil:
		search := s.ChatSearch
		localCount := 0
		for _, chat := range search.LocalChats {
			if chat.ID != 0 {
				localCount++
			}
		}
		for i, result := range flattenChatSearchResults(search) {
			switch r := result.(type) {
			case chatResult:
				if a.Action == SelectChat && a.ChatID == r.Chat.ID {
					// A chat can appear in both local and public results.
					prefix := "chat-search:local:"
					if i >= localCount {
						prefix = "chat-search:pub:"
					}
					if hit.ID == fmt.Sprintf("%s%d", prefix, r.Chat.ID) {
						search.Selected = i
						return true
					}
				}
			case messageResult:
				if a.Action == SelectMessage && a.ChatID == r.Message.ChatID && a.MessageID == r.Message.ID && hit.ID == fmt.Sprintf("chat-search:msg:%d:%d", a.ChatID, a.MessageID) {
					search.Selected = i
					return true
				}
			}
		}
	case s.ChatActions != nil:
		menu := s.ChatActions
		if menu.Working || a.ChatID != menu.ChatID {
			return false
		}
		chatIndex := chatIndex(s.Chats, menu.ChatID)
		if chatIndex < 0 {
			return false
		}
		for i, item := range ChatActionMenuItems(s.Chats[chatIndex], menu) {
			if a.Action == item.Action {
				menu.Selected = i
				return true
			}
		}
	case s.Topics != nil:
		p := s.Topics
		if i := match(topicsRows(p)); i >= 0 {
			p.Selected = i
			return true
		}
	case s.Members != nil:
		m := s.Members
		if m.Detail != nil {
			rows, _ := memberDetailRows(m, nil)
			for i, row := range rows {
				if rowSelectable(row) && row.Action == a {
					// Headers are not selectable; count actionable rows instead.
					selected := 0
					for _, previous := range rows[:i] {
						if rowSelectable(previous) {
							selected++
						}
					}
					m.Detail.Selected = selected
					return true
				}
			}
		} else if !m.Loading && a.Action == OpenMemberDetail && a.ChatID == m.ChatID {
			for i, member := range m.Results {
				if member.User.ID == a.UserID {
					m.Selected = i
					return true
				}
			}
		}
	case s.Administration != nil:
		admin := s.Administration
		if admin.Working {
			return false
		}
		index := 0
		for _, item := range AdministrationMenuItems(admin) {
			if !item.Header && item.Action.Action != NoAction {
				if item.Action == a {
					admin.Selected = index
					return true
				}
				index++
			}
		}
	case s.ChatSettings != nil:
		settings := s.ChatSettings
		if settings.Working {
			return false
		}
		for i, item := range ChatSettingsMenuItems(settings) {
			if !item.Header && item.Action == a && a.Action != NoAction {
				settings.Selected = i
				return true
			}
		}
	case s.InviteLinks != nil:
		links := s.InviteLinks
		if links.Working {
			return false
		}
		for i, item := range InviteLinkMenuItems(links) {
			if item.Action == a {
				if links.DetailURL == "" {
					links.Selected = i
				} else {
					links.DetailSelected = i
				}
				return true
			}
		}
	case s.PinnedMessages != nil:
		p := s.PinnedMessages
		if a.Action == SelectMessage && a.ChatID == p.ChatID {
			for i, message := range p.Results {
				if message.ID == a.MessageID {
					p.Selected = i
					return true
				}
			}
		}
	case s.MessageSearch != nil:
		p := s.MessageSearch
		if a.Action == SelectMessage && a.ChatID == p.ChatID && p.JumpMessageID == 0 {
			for i, message := range p.Results {
				if message.ID == a.MessageID {
					p.Selected = i
					return true
				}
			}
		}
	case s.StickerPicker != nil:
		p := s.StickerPicker
		if !p.Loading && p.Error == nil && a.Action == StickerActivate && a.RequestID == p.RequestID {
			for i, sticker := range p.Catalog {
				if sticker.File.ID == a.StickerFileID {
					p.Selected = i
					return true
				}
			}
		}
	case s.MessageMenu != nil:
		menu := s.MessageMenu
		if menu.JumpRequestID != 0 || menu.Loading || menu.Error != nil {
			return false
		}
		rows := messageActionRows(menu)
		if menu.LinkAction != NoAction {
			rows = messageLinkRows(menu)
		}
		if i := match(rows); i >= 0 {
			if menu.LinkAction != NoAction {
				menu.LinkSelected = i
			} else {
				menu.Selected = i
			}
			return true
		}
	case s.ReactionPicker != nil:
		p := s.ReactionPicker
		if a.ChatID == p.ChatID && a.MessageID == p.MessageID {
			if i := match(reactionRows(p)); i >= 0 {
				p.Selected = i
				return true
			}
		}
	case s.ForwardPicker != nil:
		p := s.ForwardPicker
		if a.Action == Activate {
			if i := chatIndex(s.Chats, a.ChatID); i >= 0 {
				p.SelectedChat = i
				return true
			}
		}
	case s.Modal != nil || s.PhotoSend != nil:
		return false
	default:
		if a.Action == FocusChat {
			if i := chatIndex(s.Chats, a.ChatID); i >= 0 && focusVisible(*s, FocusChats) {
				s.FocusedChat = i
				s.Focus = FocusChats
				return true
			}
		}
		if s.DetailsOpen && a.Action != NoAction {
			if chat, ok := detailsChat(*s); ok {
				for i, item := range DetailsActionItems(chat) {
					if a.Action == item.Action {
						s.DetailsSelected = i
						s.Focus = FocusDetails
						return true
					}
				}
			}
		}
	}
	return false
}

// Keep list pagination aligned with keyboard selection when a mouse focus
// lands on the last loaded row.
func paginateMouseListFocus(s *State) []Effect {
	switch {
	case s.Topics != nil && s.Topics.Selected == len(s.Topics.Results) && !s.Topics.Done && !s.Topics.Loading:
		return requestNextTopicsPage(s)
	case s.Members != nil && s.Members.Detail == nil:
		return maybePaginateMembers(s)
	case s.InviteLinks != nil && s.InviteLinks.DetailURL == "" && !s.InviteLinks.Confirming:
		return maybePaginateInviteLinks(s)
	case s.PinnedMessages != nil:
		return maybePaginatePinnedMessages(s)
	case s.MessageSearch != nil:
		return maybePaginateMessageSearch(s)
	default:
		return nil
	}
}

func activateMouseListRow(a ActionReceived) ActionReceived {
	switch a.Action {
	case FocusChat:
		return ActionReceived{Action: OpenChat}
	case SelectMessage:
		return ActionReceived{Action: Activate}
	default:
		return a
	}
}
