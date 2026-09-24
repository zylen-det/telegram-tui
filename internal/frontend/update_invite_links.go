package frontend

import (
	"time"

	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/telegram"
)

// InviteLinkMenuItem is one keyboard and mouse action in the single links modal.
type InviteLinkMenuItem struct {
	Label  string
	Action ActionReceived
}

func InviteLinkMenuItems(links *InviteLinksState) []InviteLinkMenuItem {
	if links == nil {
		return nil
	}
	chatID := links.ChatID
	if links.Confirming {
		return []InviteLinkMenuItem{
			{Label: "Cancel", Action: ActionReceived{Action: CancelRevokeInviteLink, ChatID: chatID}},
			{Label: "Confirm revoke", Action: ActionReceived{Action: ConfirmRevokeInviteLink, ChatID: chatID, InviteURL: links.DetailURL}},
		}
	}
	if links.DetailURL != "" {
		label := "Revoke link"
		if links.Primary != nil && links.Primary.URL == links.DetailURL {
			label = "Replace primary link"
		}
		return []InviteLinkMenuItem{
			{Label: "‹ Back", Action: ActionReceived{Action: Close, ChatID: chatID}},
			{Label: "Copy link", Action: ActionReceived{Action: CopyInviteLink, ChatID: chatID, InviteURL: links.DetailURL}},
			{Label: label, Action: ActionReceived{Action: RevokeInviteLink, ChatID: chatID, InviteURL: links.DetailURL}},
		}
	}
	if links.Error != nil {
		return []InviteLinkMenuItem{{Label: "Retry", Action: ActionReceived{Action: Retry, ChatID: chatID}}}
	}
	if links.Loading && len(links.Links) == 0 && links.Primary == nil {
		return nil
	}
	items := []InviteLinkMenuItem{{Label: "Create link", Action: ActionReceived{Action: CreateInviteLink, ChatID: chatID}}}
	if links.Primary != nil && links.Primary.URL != "" {
		items = append(items, InviteLinkMenuItem{Label: "Primary link", Action: ActionReceived{Action: OpenInviteLinkDetail, ChatID: chatID, InviteURL: links.Primary.URL}})
	}
	for _, link := range links.Links {
		if link.URL == "" || link.IsRevoked || (links.Primary != nil && link.URL == links.Primary.URL) {
			continue
		}
		label := link.Name
		if label == "" {
			label = "Invite link"
		}
		items = append(items, InviteLinkMenuItem{Label: label, Action: ActionReceived{Action: OpenInviteLinkDetail, ChatID: chatID, InviteURL: link.URL}})
	}
	return items
}

func openInviteLinks(state *State) []Effect {
	chat, ok := detailsChat(*state)
	if !ok || state.Focus != FocusDetails {
		return nil
	}
	chatID := chat.ID
	if !chat.CanManageInviteLinks || (chat.Kind != domain.ChatBasicGroup && chat.Kind != domain.ChatSupergroup && chat.Kind != domain.ChatChannel) {
		return nil
	}
	requestID := allocateRequestID(state)
	state.InviteLinks = &InviteLinksState{RequestID: requestID, ChatID: chatID, PreviousFocus: FocusDetails, Loading: true}
	state.DetailsSelected = 2
	state.Focus = FocusInviteLinks
	return []Effect{LoadInviteLinksCommand{RequestID: requestID, ChatID: chatID, Cursor: telegram.InviteLinkCursor{Limit: pageSize}}}
}

func reduceInviteLinksAction(state *State, event ActionReceived) []Effect {
	links := state.InviteLinks
	if links == nil || state.Focus != FocusInviteLinks || (event.ChatID != 0 && event.ChatID != links.ChatID) {
		return nil
	}
	if links.Working {
		if event.Action == Close {
			state.Focus = links.PreviousFocus
			state.InviteLinks = nil
		}
		return nil
	}
	if event.Action != SelectNext && event.Action != SelectPrevious && event.Action != Activate {
		links.Notice = ""
	}
	items := InviteLinkMenuItems(links)
	switch event.Action {
	case Close:
		if links.Confirming {
			links.Confirming = false
			links.DetailSelected = 2
			return nil
		}
		if links.DetailURL != "" {
			links.DetailURL = ""
			links.Selected = 0
			return nil
		}
		state.Focus = links.PreviousFocus
		state.InviteLinks = nil
		return nil
	case SelectNext, SelectPrevious:
		if len(items) == 0 {
			return nil
		}
		selected := &links.Selected
		if links.DetailURL != "" || links.Confirming {
			selected = &links.DetailSelected
		}
		delta := 1
		if event.Action == SelectPrevious {
			delta = -1
		}
		*selected = max(0, min(len(items)-1, *selected+delta))
		if links.DetailURL == "" && !links.Confirming {
			return maybePaginateInviteLinks(state)
		}
		return nil
	case Activate:
		if len(items) == 0 {
			return nil
		}
		selected := links.Selected
		if links.DetailURL != "" || links.Confirming {
			selected = links.DetailSelected
		}
		selected = max(0, min(len(items)-1, selected))
		return reduceInviteLinksAction(state, items[selected].Action)
	case Retry:
		if links.DetailURL == "" && links.Error != nil {
			return reloadInviteLinks(state)
		}
	case OpenInviteLinkDetail:
		if links.DetailURL != "" || event.InviteURL == "" || !inviteLinkKnown(links, event.InviteURL) {
			return nil
		}
		links.DetailURL = event.InviteURL
		links.DetailSelected = 0
	case CreateInviteLink:
		if links.DetailURL != "" || links.Loading || links.Error != nil {
			return nil
		}
		requestID := allocateRequestID(state)
		links.RequestID = requestID
		links.Working = true
		return []Effect{CreateInviteLinkCommand{RequestID: requestID, ChatID: links.ChatID}}
	case CopyInviteLink:
		if links.DetailURL == "" || event.InviteURL != links.DetailURL {
			return nil
		}
		requestID := allocateRequestID(state)
		links.RequestID = requestID
		links.Working = true
		return []Effect{CopyInviteLinkCommand{RequestID: requestID, ChatID: links.ChatID, URL: links.DetailURL}}
	case RevokeInviteLink:
		if links.DetailURL == "" || event.InviteURL != links.DetailURL {
			return nil
		}
		links.Confirming = true
		links.DetailSelected = 0
	case CancelRevokeInviteLink:
		links.Confirming = false
		links.DetailSelected = 2
	case ConfirmRevokeInviteLink:
		if !links.Confirming || links.DetailURL == "" || event.InviteURL != links.DetailURL {
			return nil
		}
		requestID := allocateRequestID(state)
		links.RequestID = requestID
		links.Working = true
		links.Confirming = false
		return []Effect{RevokeInviteLinkCommand{RequestID: requestID, ChatID: links.ChatID, URL: links.DetailURL}}
	}
	return nil
}

func inviteLinkKnown(links *InviteLinksState, url string) bool {
	if links.Primary != nil && links.Primary.URL == url {
		return true
	}
	for _, link := range links.Links {
		if link.URL == url {
			return true
		}
	}
	return false
}

func maybePaginateInviteLinks(state *State) []Effect {
	links := state.InviteLinks
	items := InviteLinkMenuItems(links)
	if links == nil || links.Loading || links.Working || links.Done || len(items) == 0 || links.Selected != len(items)-1 {
		return nil
	}
	requestID := allocateRequestID(state)
	links.RequestID = requestID
	links.Loading = true
	return []Effect{LoadInviteLinksCommand{RequestID: requestID, ChatID: links.ChatID, Cursor: links.NextCursor}}
}

func reloadInviteLinks(state *State) []Effect {
	links := state.InviteLinks
	if links == nil {
		return nil
	}
	requestID := allocateRequestID(state)
	*links = InviteLinksState{RequestID: requestID, ChatID: links.ChatID, PreviousFocus: links.PreviousFocus, Loading: true}
	return []Effect{LoadInviteLinksCommand{RequestID: requestID, ChatID: links.ChatID, Cursor: telegram.InviteLinkCursor{Limit: pageSize}}}
}

func reduceInviteLinksLoaded(state *State, event InviteLinksLoaded) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || !links.Loading {
		return nil
	}
	if links.Primary == nil && event.Page.Primary != nil {
		primary := *event.Page.Primary
		links.Primary = &primary
	}
	seen := make(map[string]struct{}, len(links.Links)+len(event.Page.Links))
	if links.Primary != nil {
		seen[links.Primary.URL] = struct{}{}
	}
	for _, link := range links.Links {
		seen[link.URL] = struct{}{}
	}
	for _, link := range event.Page.Links {
		if link.URL == "" || link.IsRevoked {
			continue
		}
		if _, exists := seen[link.URL]; exists {
			continue
		}
		seen[link.URL] = struct{}{}
		links.Links = append(links.Links, link)
	}
	links.NextCursor = event.Page.NextCursor
	if links.NextCursor.Limit <= 0 {
		links.NextCursor.Limit = pageSize
	}
	links.Done = event.Page.Done
	links.Loading = false
	links.Error = nil
	items := InviteLinkMenuItems(links)
	links.Selected = max(0, min(len(items)-1, links.Selected))
	return nil
}

func reduceInviteLinksLoadFailed(state *State, event InviteLinksLoadFailed) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || !links.Loading {
		return nil
	}
	links.Loading = false
	links.Error = &domain.AppError{Kind: domain.ErrorNetwork, Op: "load invite links", Message: "Could not load invite links"}
	return nil
}

func reduceInviteLinkCreated(state *State, event InviteLinkCreated) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || !links.Working {
		return nil
	}
	links.Working = false
	if event.Link.URL == "" {
		return reduceInviteLinkCreateFailed(state, InviteLinkCreateFailed{RequestID: event.RequestID, ChatID: event.ChatID})
	}
	links.Links = append([]telegram.InviteLink{event.Link}, links.Links...)
	links.DetailURL = event.Link.URL
	links.DetailSelected = 1
	links.Notice = "Invite link created"
	setToast(state, domain.AppError{Message: "Invite link created"}, 2*time.Second)
	return nil
}

func reduceInviteLinkCreateFailed(state *State, event InviteLinkCreateFailed) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID {
		return nil
	}
	links.Working = false
	links.Notice = "Could not create invite link"
	setToast(state, domain.AppError{Kind: domain.ErrorNetwork, Op: "create invite link", Message: "Could not create invite link"}, 3*time.Second)
	return nil
}

func reduceInviteLinkRevoked(state *State, event InviteLinkRevoked) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || links.DetailURL != event.URL || !links.Working {
		return nil
	}
	setToast(state, domain.AppError{Message: "Invite link revoked"}, 2*time.Second)
	commands := reloadInviteLinks(state)
	state.InviteLinks.Notice = "Invite link revoked"
	return commands
}

func reduceInviteLinkRevokeFailed(state *State, event InviteLinkRevokeFailed) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || links.DetailURL != event.URL || !links.Working {
		return nil
	}
	links.Working = false
	links.Notice = "Could not revoke invite link"
	setToast(state, domain.AppError{Kind: domain.ErrorNetwork, Op: "revoke invite link", Message: "Could not revoke invite link"}, 3*time.Second)
	return nil
}

func reduceInviteLinkCopied(state *State, event InviteLinkCopied) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || links.DetailURL != event.URL || !links.Working {
		return nil
	}
	links.Working = false
	links.Notice = "Invite link copied"
	setToast(state, domain.AppError{Message: "Invite link copied"}, 2*time.Second)
	return nil
}

func reduceInviteLinkCopyFailed(state *State, event InviteLinkCopyFailed) []Effect {
	links := state.InviteLinks
	if links == nil || links.RequestID != event.RequestID || links.ChatID != event.ChatID || links.DetailURL != event.URL || !links.Working {
		return nil
	}
	links.Working = false
	links.Notice = "Could not copy invite link"
	setToast(state, domain.AppError{Kind: domain.ErrorNetwork, Op: "copy invite link", Message: "Could not copy invite link"}, 3*time.Second)
	return nil
}
