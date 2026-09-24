package frontend

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

type AdministrationMenuItem struct {
	Label  string
	Action ActionReceived
	Header bool
}

func openGroupPermissions(state *State) []Effect {
	chat, ok := detailsChat(*state)
	if state.Focus != FocusDetails || !ok {
		return nil
	}
	if (chat.Kind != domain.ChatBasicGroup && chat.Kind != domain.ChatSupergroup) || !chat.CanRestrictMembers {
		return nil
	}
	id := allocateRequestID(state)
	state.Administration = &AdministrationState{RequestID: id, ChatID: chat.ID, PreviousFocus: FocusDetails, Mode: AdministrationDefaultPermissions, IsForum: chat.IsForum, Loading: true}
	state.Focus = FocusAdministration
	return []Effect{LoadAdministrationCommand{RequestID: id, ChatID: chat.ID}}
}

func openMemberAdministration(state *State) []Effect {
	if state.Members == nil || state.Members.Detail == nil || !state.Members.Detail.CanManageInChat || state.Focus != FocusMembers {
		return nil
	}
	chatID := state.Members.ChatID
	userID := state.Members.Detail.UserID
	id := allocateRequestID(state)
	isForum := false
	if index := chatIndex(state.Chats, chatID); index >= 0 {
		isForum = state.Chats[index].IsForum
	}
	state.Administration = &AdministrationState{RequestID: id, ChatID: chatID, UserID: userID, PreviousFocus: FocusMembers, ReturnMembers: state.Members, Mode: AdministrationMemberMenu, IsForum: isForum, Loading: true, MemberLoading: true}
	state.Members = nil
	state.Focus = FocusAdministration
	return []Effect{LoadAdministrationCommand{RequestID: id, ChatID: chatID}, LoadMemberAdministrationCommand{RequestID: id, ChatID: chatID, UserID: userID}}
}

func AdministrationMenuItems(s *AdministrationState) []AdministrationMenuItem {
	if s == nil {
		return nil
	}
	if s.Loading || s.MemberLoading {
		return nil
	}
	if s.Error != nil {
		return []AdministrationMenuItem{{Label: "Retry", Action: adminAction(s, Retry)}}
	}
	if s.Mode == AdministrationDefaultPermissions || s.Mode == AdministrationRestrictionsEditor {
		return permissionItems(s)
	}
	if s.Mode == AdministrationConfirmation {
		return []AdministrationMenuItem{{Label: "Cancel", Action: adminAction(s, CancelAdministrationAction)}, {Label: "Confirm", Action: adminAction(s, ConfirmAdministrationAction)}}
	}
	if s.Mode == AdministrationAdminRightsEditor {
		return rightsItems(s)
	}
	st := s.MemberStatus
	if st == nil || s.Snapshot == nil || st.IsCurrentUser || st.Role == domain.ChatMemberRoleOwner {
		return nil
	}
	out := []AdministrationMenuItem{}
	if st.Role == domain.ChatMemberRoleMember || st.Role == domain.ChatMemberRoleRestricted {
		if s.Snapshot.CanPromoteMembers {
			out = append(out, AdministrationMenuItem{Label: "Promote", Action: adminActionIndex(s, 1)})
		}
	}
	if st.Role == domain.ChatMemberRoleAdministrator && st.CanBeEdited && s.Snapshot.CanPromoteMembers {
		out = append(out, AdministrationMenuItem{Label: "Edit admin rights", Action: adminActionIndex(s, 2)}, AdministrationMenuItem{Label: "Demote", Action: adminActionIndex(s, 3)})
	}
	if (st.Role == domain.ChatMemberRoleMember || st.Role == domain.ChatMemberRoleRestricted) && s.Snapshot.CanRestrictMembers && s.Snapshot.Kind == domain.ChatSupergroup {
		out = append(out, AdministrationMenuItem{Label: "Restrict", Action: adminActionIndex(s, 4)})
	}
	if st.Role == domain.ChatMemberRoleRestricted && s.Snapshot.CanRestrictMembers {
		out = append(out, AdministrationMenuItem{Label: "Unrestrict", Action: adminActionIndex(s, 5)})
	}
	if (st.Role == domain.ChatMemberRoleMember || st.Role == domain.ChatMemberRoleRestricted) && s.Snapshot.CanRestrictMembers {
		out = append(out, AdministrationMenuItem{Label: "Remove", Action: adminActionIndex(s, 6)}, AdministrationMenuItem{Label: "Ban", Action: adminActionIndex(s, 7)})
	}
	return out
}
func adminAction(s *AdministrationState, a Action) ActionReceived {
	return ActionReceived{Action: a, ChatID: s.ChatID, UserID: s.UserID}
}
func adminActionIndex(s *AdministrationState, i int) ActionReceived {
	e := adminAction(s, SelectAdministrationAction)
	e.AdminIndex = i
	return e
}
func permissionItems(s *AdministrationState) []AdministrationMenuItem {
	labels := []string{"Send basic messages", "Send audios", "Send documents", "Send photos", "Send videos", "Send video notes", "Send voice notes", "Send polls", "Send other messages", "Add link previews", "React to messages", "Edit tag", "Change info", "Invite users", "Pin messages", "Create topics"}
	out := make([]AdministrationMenuItem, 0, len(labels)+2)
	for i, l := range labels {
		v := permissionValue(s, i)
		out = append(out, AdministrationMenuItem{Label: labelChecked(l, v), Action: adminToggle(s, i)})
	}
	out = append(out, AdministrationMenuItem{Label: "Save", Action: adminAction(s, SaveAdministration)}, AdministrationMenuItem{Label: "Cancel", Action: adminAction(s, CancelAdministrationAction)})
	return out
}
func rightsItems(s *AdministrationState) []AdministrationMenuItem {
	labels := []string{"Manage chat", "Change info", "Post messages", "Edit messages", "Delete messages", "Invite users", "Restrict members", "Pin messages", "Manage topics", "Promote members", "Manage video chats", "Post stories", "Edit stories", "Delete stories", "Manage direct messages", "Manage tags", "Anonymous"}
	out := make([]AdministrationMenuItem, 0, len(labels)+2)
	for i, l := range labels {
		if rightsApplicable(s, i) {
			out = append(out, AdministrationMenuItem{Label: labelChecked(l, rightValue(s, i)), Action: adminToggle(s, i)})
		}
	}
	out = append(out, AdministrationMenuItem{Label: "Save", Action: adminAction(s, SaveAdministration)}, AdministrationMenuItem{Label: "Cancel", Action: adminAction(s, CancelAdministrationAction)})
	return out
}
func adminToggle(s *AdministrationState, i int) ActionReceived {
	e := adminAction(s, ToggleAdministrationItem)
	e.AdminIndex = i
	return e
}
func labelChecked(label string, v bool) string {
	if v {
		return "[x] " + label
	}
	return "[ ] " + label
}
func permissionValue(s *AdministrationState, i int) bool {
	v := []bool{s.EditedPermissions.CanSendBasicMessages, s.EditedPermissions.CanSendAudios, s.EditedPermissions.CanSendDocuments, s.EditedPermissions.CanSendPhotos, s.EditedPermissions.CanSendVideos, s.EditedPermissions.CanSendVideoNotes, s.EditedPermissions.CanSendVoiceNotes, s.EditedPermissions.CanSendPolls, s.EditedPermissions.CanSendOtherMessages, s.EditedPermissions.CanAddLinkPreviews, s.EditedPermissions.CanReactToMessages, s.EditedPermissions.CanEditTag, s.EditedPermissions.CanChangeInfo, s.EditedPermissions.CanInviteUsers, s.EditedPermissions.CanPinMessages, s.EditedPermissions.CanCreateTopics}
	if i >= 0 && i < len(v) {
		return v[i]
	}
	return false
}
func rightValue(s *AdministrationState, i int) bool {
	v := []bool{s.EditedRights.CanManageChat, s.EditedRights.CanChangeInfo, s.EditedRights.CanPostMessages, s.EditedRights.CanEditMessages, s.EditedRights.CanDeleteMessages, s.EditedRights.CanInviteUsers, s.EditedRights.CanRestrictMembers, s.EditedRights.CanPinMessages, s.EditedRights.CanManageTopics, s.EditedRights.CanPromoteMembers, s.EditedRights.CanManageVideoChats, s.EditedRights.CanPostStories, s.EditedRights.CanEditStories, s.EditedRights.CanDeleteStories, s.EditedRights.CanManageDirectMessages, s.EditedRights.CanManageTags, s.EditedRights.IsAnonymous}
	if i >= 0 && i < len(v) {
		return v[i]
	}
	return false
}
func rightsApplicable(s *AdministrationState, i int) bool {
	if s.Snapshot == nil {
		return false
	}
	k := s.Snapshot.Kind
	if (i == 2 || i == 3 || i == 14) && k != domain.ChatChannel {
		return false
	}
	if (i == 7 || i == 15) && (k != domain.ChatBasicGroup && k != domain.ChatSupergroup) {
		return false
	}
	if i == 8 && (k != domain.ChatSupergroup || !s.IsForum) {
		return false
	}
	if (i >= 11 && i <= 13) && k == domain.ChatBasicGroup {
		return false
	}
	if i == 16 && k != domain.ChatSupergroup {
		return false
	}
	return true
}

func reduceAdministrationAction(state *State, e ActionReceived) []Effect {
	s := state.Administration
	if s == nil || state.Focus != FocusAdministration {
		return nil
	}
	if (e.ChatID != 0 && e.ChatID != s.ChatID) || (e.UserID != 0 && e.UserID != s.UserID) {
		return nil
	}
	if e.Action == Close || (e.Action == CancelAdministrationAction && s.Error == nil && !s.Loading && !s.MemberLoading) {
		if s.Mode == AdministrationConfirmation {
			s.Mode = AdministrationMemberMenu
			s.Selected = 0
			s.PendingAction = 0
			return nil
		}
		if s.Mode == AdministrationDefaultPermissions {
			state.Focus = s.PreviousFocus
			state.Administration = nil
			return nil
		}
		if s.Mode != AdministrationMemberMenu {
			s.Mode = AdministrationMemberMenu
			s.Selected = 0
			s.PendingAction = 0
			return nil
		}
		state.Focus = s.PreviousFocus
		state.Administration = nil
		if s.ReturnMembers != nil {
			state.Members = s.ReturnMembers
			state.Focus = FocusMembers
		}
		return nil
	}
	if s.Loading || s.MemberLoading || s.Working {
		return nil
	}
	if s.Error != nil {
		if e.Action != Retry && e.Action != Activate {
			return nil
		}
	}
	if e.Action == Retry && s.Error != nil {
		requestID := allocateRequestID(state)
		s.RequestID = requestID
		s.Loading = true
		s.MemberLoading = s.UserID != 0
		s.Error = nil
		s.Snapshot = nil
		s.MemberStatus = nil
		s.Selected = 0
		commands := []Effect{LoadAdministrationCommand{RequestID: requestID, ChatID: s.ChatID}}
		if s.UserID != 0 {
			commands = append(commands, LoadMemberAdministrationCommand{RequestID: requestID, ChatID: s.ChatID, UserID: s.UserID})
		}
		return commands
	}
	items := AdministrationMenuItems(s)
	if e.Action == SelectNext || e.Action == SelectPrevious {
		n := 0
		for _, v := range items {
			if !v.Header {
				n++
			}
		}
		if n > 0 {
			if e.Action == SelectNext {
				s.Selected = (s.Selected + 1) % n
			} else {
				s.Selected = (s.Selected + n - 1) % n
			}
		}
		return nil
	}
	if e.Action == Activate {
		n := -1
		for _, v := range items {
			if v.Header {
				continue
			}
			n++
			if n == s.Selected {
				return reduceAdministrationAction(state, v.Action)
			}
		}
		return nil
	}
	switch e.Action {
	case ToggleAdministrationItem:
		if s.Mode == AdministrationAdminRightsEditor && !rightsApplicable(s, e.AdminIndex) {
			return nil
		}
		toggleAdministration(s, e.AdminIndex)
	case SelectAdministrationAction:
		if s.Mode == AdministrationMemberMenu {
			allowed := false
			for _, item := range items {
				if item.Action.AdminIndex == e.AdminIndex {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil
			}
			switch e.AdminIndex {
			case 1:
				s.Mode = AdministrationAdminRightsEditor
				s.EditedRights = telegram.AdministratorRights{CanManageChat: true}
			case 2:
				s.Mode = AdministrationAdminRightsEditor
				s.EditedRights = s.MemberStatus.Rights
			case 3, 5, 6, 7:
				s.Mode = AdministrationConfirmation
				s.PendingAction = map[int]telegram.MemberAdministrationAction{3: telegram.MemberAdministrationDemote, 5: telegram.MemberAdministrationUnrestrict, 6: telegram.MemberAdministrationRemove, 7: telegram.MemberAdministrationBan}[e.AdminIndex]
			case 4:
				s.Mode = AdministrationRestrictionsEditor
				if s.MemberStatus != nil && s.MemberStatus.Role == domain.ChatMemberRoleRestricted {
					s.EditedPermissions = s.MemberStatus.Permissions
				} else {
					s.EditedPermissions = telegram.ChatPermissions{}
				}
			}
			s.Selected = 0
		}
	case SaveAdministration:
		if s.Mode != AdministrationDefaultPermissions && s.Mode != AdministrationAdminRightsEditor && s.Mode != AdministrationRestrictionsEditor {
			return nil
		}
		id := allocateRequestID(state)
		s.RequestID = id
		s.Working = true
		if s.Mode == AdministrationDefaultPermissions {
			return []Effect{SetDefaultChatPermissionsCommand{RequestID: id, ChatID: s.ChatID, Permissions: s.EditedPermissions}}
		}
		action := telegram.MemberAdministrationPromote
		if s.Mode == AdministrationRestrictionsEditor {
			action = telegram.MemberAdministrationRestrict
		}
		s.PendingAction = action
		return []Effect{ApplyMemberAdministrationCommand{RequestID: id, Request: telegram.MemberAdministrationRequest{ChatID: s.ChatID, UserID: s.UserID, Action: action, Rights: s.EditedRights, Permissions: s.EditedPermissions}}}
	case ConfirmAdministrationAction:
		if s.Mode != AdministrationConfirmation || s.PendingAction == 0 {
			return nil
		}
		id := allocateRequestID(state)
		s.RequestID = id
		s.Working = true
		return []Effect{ApplyMemberAdministrationCommand{RequestID: id, Request: telegram.MemberAdministrationRequest{ChatID: s.ChatID, UserID: s.UserID, Action: s.PendingAction}}}
	}
	return nil
}
func toggleAdministration(s *AdministrationState, i int) {
	if s.Mode == AdministrationDefaultPermissions || s.Mode == AdministrationRestrictionsEditor {
		v := []*bool{&s.EditedPermissions.CanSendBasicMessages, &s.EditedPermissions.CanSendAudios, &s.EditedPermissions.CanSendDocuments, &s.EditedPermissions.CanSendPhotos, &s.EditedPermissions.CanSendVideos, &s.EditedPermissions.CanSendVideoNotes, &s.EditedPermissions.CanSendVoiceNotes, &s.EditedPermissions.CanSendPolls, &s.EditedPermissions.CanSendOtherMessages, &s.EditedPermissions.CanAddLinkPreviews, &s.EditedPermissions.CanReactToMessages, &s.EditedPermissions.CanEditTag, &s.EditedPermissions.CanChangeInfo, &s.EditedPermissions.CanInviteUsers, &s.EditedPermissions.CanPinMessages, &s.EditedPermissions.CanCreateTopics}
		if i >= 0 && i < len(v) {
			if s.Mode == AdministrationRestrictionsEditor && !*v[i] && !permissionValueSnapshot(s.Snapshot, i) {
				return
			}
			*v[i] = !*v[i]
		}
	}
	if s.Mode == AdministrationAdminRightsEditor {
		v := []*bool{&s.EditedRights.CanManageChat, &s.EditedRights.CanChangeInfo, &s.EditedRights.CanPostMessages, &s.EditedRights.CanEditMessages, &s.EditedRights.CanDeleteMessages, &s.EditedRights.CanInviteUsers, &s.EditedRights.CanRestrictMembers, &s.EditedRights.CanPinMessages, &s.EditedRights.CanManageTopics, &s.EditedRights.CanPromoteMembers, &s.EditedRights.CanManageVideoChats, &s.EditedRights.CanPostStories, &s.EditedRights.CanEditStories, &s.EditedRights.CanDeleteStories, &s.EditedRights.CanManageDirectMessages, &s.EditedRights.CanManageTags, &s.EditedRights.IsAnonymous}
		if i >= 0 && i < len(v) {
			if !rightValue(s, i) && s.Snapshot != nil && !s.Snapshot.IsOwner && !rightOwn(s.Snapshot, i) {
				return
			}
			*v[i] = !*v[i]
		}
	}
}
func permissionValueSnapshot(s *telegram.AdministrationSnapshot, i int) bool {
	if s == nil {
		return false
	}
	v := []bool{s.DefaultPermissions.CanSendBasicMessages, s.DefaultPermissions.CanSendAudios, s.DefaultPermissions.CanSendDocuments, s.DefaultPermissions.CanSendPhotos, s.DefaultPermissions.CanSendVideos, s.DefaultPermissions.CanSendVideoNotes, s.DefaultPermissions.CanSendVoiceNotes, s.DefaultPermissions.CanSendPolls, s.DefaultPermissions.CanSendOtherMessages, s.DefaultPermissions.CanAddLinkPreviews, s.DefaultPermissions.CanReactToMessages, s.DefaultPermissions.CanEditTag, s.DefaultPermissions.CanChangeInfo, s.DefaultPermissions.CanInviteUsers, s.DefaultPermissions.CanPinMessages, s.DefaultPermissions.CanCreateTopics}
	return i >= 0 && i < len(v) && v[i]
}
func rightOwn(s *telegram.AdministrationSnapshot, i int) bool {
	v := []bool{s.OwnRights.CanManageChat, s.OwnRights.CanChangeInfo, s.OwnRights.CanPostMessages, s.OwnRights.CanEditMessages, s.OwnRights.CanDeleteMessages, s.OwnRights.CanInviteUsers, s.OwnRights.CanRestrictMembers, s.OwnRights.CanPinMessages, s.OwnRights.CanManageTopics, s.OwnRights.CanPromoteMembers, s.OwnRights.CanManageVideoChats, s.OwnRights.CanPostStories, s.OwnRights.CanEditStories, s.OwnRights.CanDeleteStories, s.OwnRights.CanManageDirectMessages, s.OwnRights.CanManageTags, s.OwnRights.IsAnonymous}
	return i >= 0 && i < len(v) && v[i]
}
func reduceAdministrationLoaded(state *State, e AdministrationLoaded) []Effect {
	s := state.Administration
	if s == nil || s.RequestID != e.RequestID || s.ChatID != e.ChatID || !s.Loading || !administrationChatActive(*state, e.ChatID) {
		return nil
	}
	s.Snapshot = &e.Snapshot
	s.Loading = false
	s.EditedPermissions = e.Snapshot.DefaultPermissions
	if s.Mode == AdministrationDefaultPermissions && (e.Snapshot.Kind == domain.ChatChannel || !e.Snapshot.CanRestrictMembers) {
		s.Error = &domain.AppError{Kind: domain.ErrorInternal, Op: "load administration", Message: "Group permissions are unavailable"}
	}
	return nil
}
func reduceMemberAdministrationLoaded(state *State, e MemberAdministrationLoaded) []Effect {
	s := state.Administration
	if s == nil || s.RequestID != e.RequestID || s.ChatID != e.ChatID || s.UserID != e.UserID || !s.MemberLoading || !administrationChatActive(*state, e.ChatID) {
		return nil
	}
	v := e.Status
	s.MemberStatus = &v
	s.MemberLoading = false
	if s.Mode == AdministrationRestrictionsEditor && v.Role == domain.ChatMemberRoleRestricted {
		s.EditedPermissions = v.Permissions
	}
	return nil
}
func adminToast(state *State, msg string) {
	setToast(state, domain.AppError{Message: msg}, 2*time.Second)
}
