package frontend

import (
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/avatar"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

func openMembers(state *State) []Effect {
	chat, ok := detailsChat(*state)
	if !ok || state.Focus != FocusDetails {
		return nil
	}
	chatID := chat.ID
	if chat.Kind != domain.ChatBasicGroup && chat.Kind != domain.ChatSupergroup && chat.Kind != domain.ChatChannel {
		return nil
	}
	requestID := allocateRequestID(state)
	state.Members = &MembersState{
		RequestID:     requestID,
		ChatID:        chatID,
		PreviousFocus: state.Focus,
		Loading:       true,
	}
	if count := detailsActionCount(chat); count > 1 {
		state.DetailsSelected = 1
	}
	state.Focus = FocusMembers
	return []Effect{LoadMembers{
		RequestID: requestID,
		ChatID:    chatID,
		Cursor:    telegram.MemberCursor{Limit: pageSize},
	}}
}

// openUserInfo opens the single Members modal directly for one user, backed
// by an explicit user fetch instead of the member list.
func openUserInfo(state *State, chatID domain.ChatID, userID domain.UserID, previousFocus Focus) []Effect {
	if userID == 0 {
		return nil
	}
	requestID := allocateRequestID(state)
	state.Members = &MembersState{
		RequestID:     requestID,
		ChatID:        chatID,
		PreviousFocus: previousFocus,
		Loading:       true,
		Done:          true,
		Single:        true,
	}
	state.Focus = FocusMembers
	return []Effect{LoadUserInfo{RequestID: requestID, ChatID: chatID, UserID: userID}}
}

func reduceUserInfoLoaded(state *State, event UserInfoLoaded) []Effect {
	members := state.Members
	if members == nil || !members.Single || members.RequestID != event.RequestID || members.ChatID != event.ChatID {
		return nil
	}
	if event.User.ID == 0 {
		return reduceUserInfoFailed(state, UserInfoLoadFailed{RequestID: event.RequestID, ChatID: event.ChatID, UserID: event.UserID, Error: userInfoError()})
	}
	detail := &MemberDetail{
		UserID:   event.User.ID,
		Name:     event.User.Name,
		Username: event.User.Username,
		Avatar:   event.User.Avatar,
	}
	if event.User.Avatar.UniqueID != "" {
		detail.AvatarKey = avatar.CacheKey(event.User.Avatar, avatar.RoleChatList)
	}
	members.Detail = detail
	members.Loading = false
	members.Error = nil
	state.Focus = FocusMembers
	return requestMissingDetailAvatar(state, detail)
}

func reduceUserInfoFailed(state *State, event UserInfoLoadFailed) []Effect {
	members := state.Members
	if members == nil || !members.Single || members.RequestID != event.RequestID || members.ChatID != event.ChatID {
		return nil
	}
	failure := userInfoError()
	members.Error = &failure
	members.Loading = false
	state.Focus = FocusMembers
	return nil
}

func reduceMembersAction(state *State, event ActionReceived) []Effect {
	members := state.Members
	if members == nil {
		return nil
	}
	if members.Detail != nil {
		return reduceMemberDetailActionRouter(state, event)
	}
	switch event.Action {
	case Close:
		state.Focus = members.PreviousFocus
		state.Members = nil
	case SelectChat:
		state.Focus = FocusConversation
		state.Members = nil
		return reduceAction(state, event)
	case SelectMember:
		if state.Focus != FocusMembers || event.ChatID != members.ChatID {
			return nil
		}
		for index := range members.Results {
			if members.Results[index].User.ID == event.UserID {
				members.Selected = index
				return maybePaginateMembers(state)
			}
		}
	case Activate:
		if state.Focus != FocusMembers || members.Loading || len(members.Results) == 0 {
			return nil
		}
		selected := max(0, min(members.Selected, len(members.Results)-1))
		return openMemberDetail(state, members.ChatID, members.Results[selected].User.ID)
	case OpenMemberDetail:
		if state.Focus != FocusMembers {
			return nil
		}
		userID := event.UserID
		if userID == 0 && len(members.Results) > 0 {
			userID = members.Results[max(0, min(members.Selected, len(members.Results)-1))].User.ID
		}
		return openMemberDetail(state, members.ChatID, userID)
	}
	return nil
}

// reduceMemberDetailActionRouter handles keys while the modal shows one
// member's detail view. Close backs out to the list; list-only actions such
// as chat selection still apply.
func reduceMemberDetailActionRouter(state *State, event ActionReceived) []Effect {
	members := state.Members
	switch event.Action {
	case Close:
		// Single-user mode has no list behind the detail: close the modal.
		if members.Single {
			state.Focus = members.PreviousFocus
			state.Members = nil
			return nil
		}
		members.Detail = nil
		return nil
	case SelectChat:
		state.Focus = FocusConversation
		state.Members = nil
		return reduceAction(state, event)
	case CloseMemberDetail:
		if event.ChatID != 0 && event.ChatID != members.ChatID {
			return nil
		}
		if members.Single {
			state.Focus = members.PreviousFocus
			state.Members = nil
			return nil
		}
		members.Detail = nil
		return nil
	default:
		return reduceMemberDetailAction(state, event)
	}
}

func maybePaginateMembers(state *State) []Effect {
	members := state.Members
	if members == nil || members.Loading || members.Done || len(members.Results) == 0 || members.Selected != len(members.Results)-1 {
		return nil
	}
	return requestNextMembersPage(state)
}

func requestNextMembersPage(state *State) []Effect {
	members := state.Members
	if members == nil || members.Loading || members.Done {
		return nil
	}
	requestID := allocateRequestID(state)
	members.RequestID = requestID
	members.Loading = true
	members.Error = nil
	return []Effect{LoadMembers{
		RequestID: requestID,
		ChatID:    members.ChatID,
		Cursor: telegram.MemberCursor{
			Offset: members.NextOffset,
			Limit:  pageSize,
		},
	}}
}

func reduceMembersLoaded(state *State, event MembersLoaded) []Effect {
	members := state.Members
	if members == nil || members.RequestID != event.RequestID || members.ChatID != event.ChatID {
		return nil
	}
	previousOffset := members.NextOffset
	seen := make(map[domain.UserID]struct{}, len(members.Results)+len(event.Page.Members))
	for _, member := range members.Results {
		seen[member.User.ID] = struct{}{}
	}
	for _, member := range event.Page.Members {
		if member.User.ID == 0 {
			continue
		}
		if _, exists := seen[member.User.ID]; exists {
			continue
		}
		seen[member.User.ID] = struct{}{}
		members.Results = append(members.Results, member)
	}
	members.TotalCount = max(0, event.Page.TotalCount)
	members.NextOffset = max(previousOffset, event.Page.NextOffset)
	members.Done = event.Page.Done || event.Page.NextOffset <= previousOffset
	members.Loading = false
	members.Error = nil
	if len(members.Results) == 0 {
		members.Selected = 0
	} else {
		members.Selected = max(0, min(len(members.Results)-1, members.Selected))
	}
	state.Focus = FocusMembers
	if len(event.Page.Members) == 0 && !members.Done {
		return requestNextMembersPage(state)
	}
	return nil
}

func reduceMembersLoadFailed(state *State, event MembersLoadFailed) []Effect {
	members := state.Members
	if members == nil || members.RequestID != event.RequestID || members.ChatID != event.ChatID {
		return nil
	}
	failure := membersError()
	members.Error = &failure
	members.Loading = false
	state.Focus = FocusMembers
	return nil
}
