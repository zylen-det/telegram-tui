package frontend

import (
	"image"
	"strconv"
	"time"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/telegram"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func inviteLinksFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	return centeredSurfaceRectangle(bounds, min(64, bounds.Dx()), min(12, bounds.Dy())).Intersect(bounds)
}

func buildInviteLinksLayer(model ui.ViewModel, styles renderStyles) surfaceResult {
	links := model.InviteLinks
	if links == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	items := app.InviteLinkMenuItems(links)
	selected := links.Selected
	title := "Invite links"
	rows := make([]modalRowSpec, 0, len(items)+4)
	if links.DetailURL != "" {
		selected = links.DetailSelected
		title = "Link details"
		if links.Confirming {
			title = "Confirm revoke"
			rows = append(rows, modalRowSpec{Label: "Revoke this invite link?", Header: true})
			if links.Primary != nil && links.Primary.URL == links.DetailURL {
				rows = append(rows, modalRowSpec{Label: "A new primary link will be issued.", Header: true})
			}
		} else {
			rows = append(rows, modalRowSpec{Label: sanitizeDisplayString(links.DetailURL), Header: true})
			if link := visibleInviteLink(links, links.DetailURL); link != nil {
				rows = append(rows, modalRowSpec{Label: inviteLinkMeta(*link), Header: true})
			}
		}
	}
	if links.Notice != "" {
		title += " · " + sanitizeDisplayString(links.Notice)
	}
	if !links.Confirming && links.DetailURL == "" && len(items) > 0 && links.Primary == nil && len(links.Links) == 0 && !links.Loading && links.Error == nil {
		rows = append(rows, modalRowSpec{Label: "No invite links", Header: true})
	}
	headerCount := len(rows)
	for index, item := range items {
		rows = append(rows, modalRowSpec{
			ID:       "invite-link:row:" + strconv.Itoa(index),
			Label:    sanitizeDisplayString(item.Label),
			Selected: index == selected,
			Action:   item.Action,
		})
	}
	switch {
	case links.Working:
		rows = append(rows, modalRowSpec{Label: "Working..."})
	case links.Loading:
		rows = append(rows, modalRowSpec{Label: "Loading invite links..."})
	case links.Error != nil:
		rows = append(rows, modalRowSpec{Label: links.Error.Message})
	}
	capacity := max(1, bounds.Dy()-5)
	rows = windowInviteLinksRows(rows, headerCount+selected, capacity)
	return buildListModalWidth(bounds, title, rows, styles, inviteLinksFrame(bounds).Dx())
}

func visibleInviteLink(links *app.InviteLinksState, url string) *telegram.InviteLink {
	if links.Primary != nil && links.Primary.URL == url {
		return links.Primary
	}
	for index := range links.Links {
		if links.Links[index].URL == url {
			return &links.Links[index]
		}
	}
	return nil
}

func inviteLinkMeta(link telegram.InviteLink) string {
	label := "Joined: " + strconv.Itoa(max(0, link.MemberCount))
	if link.MemberLimit > 0 {
		label += " · limit " + strconv.Itoa(link.MemberLimit)
	}
	if link.ExpirationDate > 0 {
		label += " · expires " + time.Unix(link.ExpirationDate, 0).UTC().Format("2006-01-02")
	}
	if link.CreatesJoinRequest {
		label += " · approval required"
	}
	return label
}

func windowInviteLinksRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}
