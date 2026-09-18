package app

import (
	"strings"
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
)

type DetailsActionItem struct {
	Action Action
	Label  string
}

func DetailsActionItems(chat domain.Chat) []DetailsActionItem {
	if chat.ID == 0 {
		return nil
	}
	items := []DetailsActionItem{{OpenDetailsAvatar, "View image"}}
	if chat.Kind == domain.ChatBasicGroup || chat.Kind == domain.ChatSupergroup || chat.Kind == domain.ChatChannel {
		items = append(items, DetailsActionItem{OpenMembers, "Members"})
		if chat.CanManageInviteLinks {
			items = append(items, DetailsActionItem{OpenInviteLinks, "Invite links"})
		}
	}
	if (chat.Kind == domain.ChatBasicGroup || chat.Kind == domain.ChatSupergroup) && chat.CanRestrictMembers {
		items = append(items, DetailsActionItem{OpenGroupPermissions, "Group permissions"})
	}
	if (chat.Kind == domain.ChatBasicGroup || chat.Kind == domain.ChatSupergroup || chat.Kind == domain.ChatChannel) && (chat.CanChangeInfo || (chat.Kind == domain.ChatSupergroup && chat.CanRestrictMembers)) {
		items = append(items, DetailsActionItem{OpenChatSettings, "Chat settings"})
	}
	return items
}
func detailsActionCount(chat domain.Chat) int { return len(DetailsActionItems(chat)) }

func clampDetailsSelection(state *State) {
	count := 0
	if state.SelectedChat >= 0 && state.SelectedChat < len(state.Chats) {
		count = detailsActionCount(state.Chats[state.SelectedChat])
	}
	if count <= 0 {
		state.DetailsSelected = 0
		return
	}
	state.DetailsSelected = max(0, min(state.DetailsSelected, count-1))
}

func openDetailsAvatar(state State) (State, []Command) {
	if _, ok := activeChatID(state); !ok {
		return state, nil
	}
	state.DetailsSelected = 0
	chat := state.Chats[state.SelectedChat]
	requestID := allocateRequestID(&state)
	state.Modal = &ModalState{RequestID: requestID, Title: chat.Title, Ref: chat.Avatar, Loading: true, PreviousFocus: state.Focus}
	state.Focus = FocusModal
	return state, []Command{OpenAvatar{RequestID: requestID, Title: chat.Title, Ref: chat.Avatar}}
}

// MemberDetailActions returns the fixed action order for a member detail
// view. View avatar needs a reachable avatar; Copy needs a username.
func MemberDetailActions(detail *MemberDetail) []Action {
	if detail == nil {
		return nil
	}
	actions := make([]Action, 0, 6)
	if detail.Avatar.UniqueID != "" {
		actions = append(actions, ViewMemberAvatar)
	}
	if strings.TrimSpace(detail.Username) != "" {
		actions = append(actions, CopyMemberUsername)
	}
	actions = append(actions, AddMemberContact, RemoveMemberContact, BlockMember, UnblockMember)
	if detail.CanManageInChat {
		actions = append(actions, OpenMemberAdministration)
	}
	return actions
}

// memberDetailRowCount counts navigable detail rows: Back plus actions.
func memberDetailRowCount(detail *MemberDetail) int {
	return 1 + len(MemberDetailActions(detail))
}

func openMemberDetail(state State, chatID domain.ChatID, userID domain.UserID) (State, []Command) {
	members := state.Members
	if members == nil || members.ChatID != chatID {
		return state, nil
	}
	for _, member := range members.Results {
		if member.User.ID != userID {
			continue
		}
		detail := &MemberDetail{
			UserID:    userID,
			Name:      member.User.Name,
			Username:  member.User.Username,
			Role:      member.Role,
			Tag:       member.Tag,
			Avatar:    member.User.Avatar,
			IsCurrent: member.User.IsCurrent,
		}
		if index := state.SelectedChat; index >= 0 && index < len(state.Chats) && state.Chats[index].ID == chatID {
			detail.CanManageInChat = !detail.IsCurrent && member.Role != domain.ChatMemberRoleOwner && (state.Chats[index].CanRestrictMembers || state.Chats[index].CanPromoteMembers)
		}
		if member.User.Avatar.UniqueID != "" {
			detail.AvatarKey = avatar.CacheKey(member.User.Avatar, avatar.RoleChatList)
		}
		members.Detail = detail
		return state, requestMissingDetailAvatar(&state, detail)
	}
	return state, nil
}

// requestMissingDetailAvatar issues one RenderAvatar for the detail member's
// avatar when it has never been requested. Rendered cells land in the shared
// avatar cache; the detail view reads them on the next snapshot.
func requestMissingDetailAvatar(state *State, detail *MemberDetail) []Command {
	if detail == nil || detail.AvatarKey == "" || detail.Avatar.UniqueID == "" {
		return nil
	}
	if _, exists := state.Avatars[detail.AvatarKey]; exists {
		return nil
	}
	state.Avatars[detail.AvatarKey] = AvatarState{Loading: true, Ref: detail.Avatar, Role: avatar.RoleChatList, Label: detail.Name}
	return []Command{RenderAvatar{Key: detail.AvatarKey, Ref: detail.Avatar, Role: avatar.RoleChatList}}
}

func memberDetailMatches(detail *MemberDetail, requestID uint64, userID domain.UserID) bool {
	return detail != nil && detail.RequestID == requestID && detail.UserID == userID
}

func reduceMemberDetailAction(state State, event ActionReceived) (State, []Command) {
	members := state.Members
	if members == nil || members.Detail == nil {
		return state, nil
	}
	detail := members.Detail
	actions := MemberDetailActions(detail)
	switch event.Action {
	case CloseMemberDetail:
		members.Detail = nil
	case OpenMemberAdministration:
		return openMemberAdministration(state)
	case ViewMemberAvatar:
		if detail.Avatar.UniqueID == "" || detail.Working {
			return state, nil
		}
		title := detail.Name
		if title == "" {
			title = "Member"
		}
		requestID := allocateRequestID(&state)
		state.Modal = &ModalState{RequestID: requestID, Title: title, Ref: detail.Avatar, Loading: true, PreviousFocus: state.Focus}
		state.Focus = FocusModal
		return state, []Command{OpenAvatar{RequestID: requestID, Title: title, Ref: detail.Avatar}}
	case CopyMemberUsername:
		username := strings.TrimPrefix(strings.TrimSpace(detail.Username), "@")
		if username == "" || detail.Working {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		detail.RequestID = requestID
		detail.Working = true
		return state, []Command{CopyMemberUsernameCommand{RequestID: requestID, ChatID: members.ChatID, UserID: detail.UserID, Text: "@" + username}}
	case AddMemberContact:
		if detail.Working {
			return state, nil
		}
		first, last := splitMemberName(detail.Name, detail.Username)
		requestID := allocateRequestID(&state)
		detail.RequestID = requestID
		detail.Working = true
		return state, []Command{AddMemberContactCommand{RequestID: requestID, ChatID: members.ChatID, UserID: detail.UserID, FirstName: first, LastName: last}}
	case RemoveMemberContact:
		if detail.Working {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		detail.RequestID = requestID
		detail.Working = true
		return state, []Command{RemoveMemberContactCommand{RequestID: requestID, ChatID: members.ChatID, UserID: detail.UserID}}
	case BlockMember:
		if detail.Working {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		detail.RequestID = requestID
		detail.Working = true
		return state, []Command{SetMemberBlockedCommand{RequestID: requestID, ChatID: members.ChatID, UserID: detail.UserID, Blocked: true}}
	case UnblockMember:
		if detail.Working {
			return state, nil
		}
		requestID := allocateRequestID(&state)
		detail.RequestID = requestID
		detail.Working = true
		return state, []Command{SetMemberBlockedCommand{RequestID: requestID, ChatID: members.ChatID, UserID: detail.UserID, Blocked: false}}
	case SelectNext, SelectPrevious:
		if detail.Working {
			return state, nil
		}
		count := memberDetailRowCount(detail)
		if count == 0 {
			return state, nil
		}
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		detail.Selected = (detail.Selected + delta + count) % count
	case Activate:
		if detail.Working {
			return state, nil
		}
		if detail.Selected == 0 {
			// Same as the ‹ Back row: single-user mode has no list to
			// return to, so this closes the modal.
			if members.Single {
				state.Focus = members.PreviousFocus
				state.Members = nil
				return state, nil
			}
			members.Detail = nil
			return state, nil
		}
		index := detail.Selected - 1
		if index < 0 || index >= len(actions) {
			return state, nil
		}
		event.Action = actions[index]
		return reduceMemberDetailAction(state, event)
	}
	return state, nil
}

// splitMemberName derives contact first/last names from the member display
// name, falling back to the username and finally a generic label so the
// TDLib first-name requirement is always satisfied.
func splitMemberName(name, username string) (string, string) {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		if trimmed := strings.TrimPrefix(strings.TrimSpace(username), "@"); trimmed != "" {
			fields = []string{trimmed}
		} else {
			fields = []string{"Telegram"}
		}
	}
	first := fields[0]
	last := ""
	if len(fields) > 1 {
		last = strings.Join(fields[1:], " ")
	}
	return truncateRunes(first, 64), truncateRunes(last, 64)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max])
	}
	return value
}

func reduceMemberUsernameCopied(state State, event MemberUsernameCopied) (State, []Command) {
	members := state.Members
	if members == nil || !memberDetailMatches(members.Detail, event.RequestID, event.UserID) || members.ChatID != event.ChatID {
		return state, nil
	}
	members.Detail.Working = false
	setToast(&state, domain.AppError{Message: "Username copied"}, 2*time.Second)
	return state, nil
}

func reduceMemberUsernameCopyFailed(state State, event MemberUsernameCopyFailed) (State, []Command) {
	members := state.Members
	if members == nil || !memberDetailMatches(members.Detail, event.RequestID, event.UserID) || members.ChatID != event.ChatID {
		return state, nil
	}
	members.Detail.Working = false
	setToast(&state, memberCopyError(), 3*time.Second)
	return state, nil
}

func reduceMemberContactChanged(state State, event MemberContactChanged) (State, []Command) {
	members := state.Members
	if members == nil || !memberDetailMatches(members.Detail, event.RequestID, event.UserID) || members.ChatID != event.ChatID {
		return state, nil
	}
	members.Detail.Working = false
	label := "Removed from contacts"
	if event.Added {
		label = "Added to contacts"
	}
	setToast(&state, domain.AppError{Message: label}, 2*time.Second)
	return state, nil
}

func reduceMemberContactFailed(state State, event MemberContactFailed) (State, []Command) {
	members := state.Members
	if members == nil || !memberDetailMatches(members.Detail, event.RequestID, event.UserID) || members.ChatID != event.ChatID {
		return state, nil
	}
	members.Detail.Working = false
	setToast(&state, memberContactError(event.Added), 3*time.Second)
	return state, nil
}

func reduceMemberBlockChanged(state State, event MemberBlockChanged) (State, []Command) {
	members := state.Members
	if members == nil || !memberDetailMatches(members.Detail, event.RequestID, event.UserID) || members.ChatID != event.ChatID {
		return state, nil
	}
	members.Detail.Working = false
	label := "User unblocked"
	if event.Blocked {
		label = "User blocked"
	}
	setToast(&state, domain.AppError{Message: label}, 2*time.Second)
	return state, nil
}

func reduceMemberBlockFailed(state State, event MemberBlockFailed) (State, []Command) {
	members := state.Members
	if members == nil || !memberDetailMatches(members.Detail, event.RequestID, event.UserID) || members.ChatID != event.ChatID {
		return state, nil
	}
	members.Detail.Working = false
	setToast(&state, memberBlockError(event.Blocked), 3*time.Second)
	return state, nil
}
