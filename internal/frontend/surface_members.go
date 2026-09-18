package frontend

import (
	"fmt"
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/domain"
	"github.com/zylen-det/telegram-tui/internal/media/pixel"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

// memberDetailAvatarWidth/Height is the sampled avatar geometry inside the
// single Members modal: 12 half-block cells wide, 4 rows tall.
const (
	memberDetailAvatarWidth  = 12
	memberDetailAvatarHeight = 4
)

func membersFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	width := min(64, bounds.Dx())
	height := min(12, bounds.Dy())
	return centeredSurfaceRectangle(bounds, width, height).Intersect(bounds)
}

func membersTitle(members *app.MembersState) string {
	if members != nil && members.Notice != "" {
		return "Members · " + sanitizeDisplayString(members.Notice)
	}
	if members != nil && members.Detail != nil {
		name := sanitizeDisplayString(members.Detail.Name)
		if name != "" {
			return name
		}
		return "Member"
	}
	return "Members"
}

func buildMembersLayer(model ui.ViewModel, styles renderStyles, selectorView string) surfaceResult {
	if model.Members == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	width := membersFrame(bounds).Dx()
	rows := displayedMembersRows(model, styles.Dim)
	if selectorView != "" {
		return buildListModalWidth(bounds, membersTitle(model.Members), rows, styles, width, selectorView)
	}
	return buildListModalWidth(bounds, membersTitle(model.Members), rows, styles, width)
}

// displayedMembersRows is the single row source for the one Members modal:
// list rows, or the detail view's avatar/info chrome plus Back and the
// reducer-ordered actions, each reduced to the window that reaches the frame.
// dim matches the renderer's avatar faint flag so the painted header rows are
// identical wherever the rows are drawn.
func displayedMembersRows(model ui.ViewModel, dim bool) []modalRowSpec {
	if model.Members == nil {
		return nil
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	width := membersFrame(bounds).Dx()
	var rows []modalRowSpec
	var selected int
	if model.Members.Detail != nil {
		var headers int
		rows, headers = memberDetailRows(model.Members, memberAvatarLabels(model.MemberDetailAvatar, width-memberDetailChrome, dim))
		selected = headers + model.Members.Detail.Selected
	} else {
		rows = membersRows(model.Members)
		selected = model.Members.Selected
	}
	return windowMembersRows(rows, selected, max(1, model.Height-5))
}

// memberDetailChromeRows counts the non-navigable detail rows the selector
// geometry must account for: avatar rows, info headers, and the Working
// status row. Navigable options (Back plus actions) are counted separately.
func memberDetailChromeRows(detail *app.MemberDetail, hasAvatar bool) int {
	if detail == nil {
		return 0
	}
	n := 1
	if memberRoleLabel(detail.Role, detail.Tag) != "" {
		n++
	}
	if hasAvatar {
		n += memberDetailAvatarHeight
	}
	if detail.Working {
		n++
	}
	return n
}

// memberDetailChrome is the horizontal overhead around a modal row label:
// two border cells on each side plus one leading content cell.
const memberDetailChrome = 5

// memberAvatarLabels samples rendered avatar cells into centered half-block
// text rows for modal header display. An empty avatar yields no rows and the
// detail view falls back to text info.
func memberAvatarLabels(avatar pixel.Avatar, labelWidth int, dim bool) []string {
	if avatar.Width <= 0 || avatar.Height <= 0 || len(avatar.Cells) < avatar.Width*avatar.Height {
		return nil
	}
	pad := max(0, (labelWidth-memberDetailAvatarWidth)/2)
	prefix := strings.Repeat(" ", pad)
	rows := make([]string, 0, memberDetailAvatarHeight)
	for localY := 0; localY < memberDetailAvatarHeight; localY++ {
		sourceY := localY * avatar.Height / memberDetailAvatarHeight
		var row strings.Builder
		row.WriteString(prefix)
		for localX := 0; localX < memberDetailAvatarWidth; localX++ {
			sourceX := localX * avatar.Width / memberDetailAvatarWidth
			cell := avatar.Cells[sourceY*avatar.Width+sourceX]
			row.WriteString(lipgloss.NewStyle().
				Foreground(rgba(rgb{cell.Foreground.R, cell.Foreground.G, cell.Foreground.B})).
				Background(rgba(rgb{cell.Background.R, cell.Background.G, cell.Background.B})).
				Faint(dim).
				Render("▀"))
		}
		rows = append(rows, row.String())
	}
	return rows
}

func membersRows(members *app.MembersState) []modalRowSpec {
	if members == nil {
		return nil
	}
	rows := make([]modalRowSpec, 0, len(members.Results)+1)
	for index, member := range members.Results {
		rows = append(rows, modalRowSpec{
			ID:       fmt.Sprintf("member:%d", member.User.ID),
			Label:    memberResultLabel(member),
			Selected: index == members.Selected,
			Action: app.ActionReceived{
				Action: app.OpenMemberDetail,
				ChatID: members.ChatID,
				UserID: member.User.ID,
			},
		})
	}
	switch {
	case members.Loading:
		rows = append(rows, modalRowSpec{Label: "Loading members..."})
	case members.Error != nil:
		rows = append(rows, modalRowSpec{Label: members.Error.Message})
	case len(members.Results) == 0:
		rows = append(rows, modalRowSpec{Label: "No members"})
	}
	return rows
}

// memberDetailRows renders the in-modal detail view: avatar header rows,
// user info header rows, a Back row, then the action rows in reducer order.
// It also returns the leading non-navigable header count so windowing can
// offset the detail selection index.
func memberDetailRows(members *app.MembersState, avatar []string) ([]modalRowSpec, int) {
	if members == nil || members.Detail == nil {
		return nil, 0
	}
	detail := members.Detail
	rows := make([]modalRowSpec, 0, 12)
	for _, line := range avatar {
		rows = append(rows, modalRowSpec{Label: line, Header: true})
	}
	header := sanitizeDisplayString(detail.Name)
	if header == "" {
		header = "Unknown"
	}
	if username := sanitizeDisplayString(detail.Username); username != "" {
		header += " · @" + strings.TrimPrefix(username, "@")
	}
	rows = append(rows, modalRowSpec{Label: header, Header: true})
	if role := memberRoleLabel(detail.Role, detail.Tag); role != "" {
		rows = append(rows, modalRowSpec{Label: role, Header: true})
	}
	headers := len(rows)
	rows = append(rows, modalRowSpec{
		ID:       "member:back",
		Label:    "‹ Back",
		Selected: detail.Selected == 0,
		Action:   app.ActionReceived{Action: app.CloseMemberDetail, ChatID: members.ChatID},
	})
	for index, item := range app.MemberDetailActions(detail) {
		spec := modalRowSpec{Selected: detail.Selected == index+1}
		switch item {
		case app.ViewMemberAvatar:
			spec.ID = "member-action:view-avatar"
			spec.Label = "View avatar"
			spec.Action = app.ActionReceived{Action: app.ViewMemberAvatar, ChatID: members.ChatID, UserID: detail.UserID}
		case app.CopyMemberUsername:
			spec.ID = "member-action:copy-username"
			spec.Label = "Copy username"
			spec.Action = app.ActionReceived{Action: app.CopyMemberUsername, ChatID: members.ChatID, UserID: detail.UserID}
		case app.AddMemberContact:
			spec.ID = "member-action:add-contact"
			spec.Label = "Add to contacts"
			spec.Action = app.ActionReceived{Action: app.AddMemberContact, ChatID: members.ChatID, UserID: detail.UserID}
		case app.RemoveMemberContact:
			spec.ID = "member-action:remove-contact"
			spec.Label = "Remove from contacts"
			spec.Action = app.ActionReceived{Action: app.RemoveMemberContact, ChatID: members.ChatID, UserID: detail.UserID}
		case app.BlockMember:
			spec.ID = "member-action:block"
			spec.Label = "Block user"
			spec.Action = app.ActionReceived{Action: app.BlockMember, ChatID: members.ChatID, UserID: detail.UserID}
		case app.UnblockMember:
			spec.ID = "member-action:unblock"
			spec.Label = "Unblock user"
			spec.Action = app.ActionReceived{Action: app.UnblockMember, ChatID: members.ChatID, UserID: detail.UserID}
		case app.OpenMemberAdministration:
			spec.ID = "member-action:manage-in-chat"
			spec.Label = "Manage in chat"
			spec.Action = app.ActionReceived{Action: app.OpenMemberAdministration, ChatID: members.ChatID, UserID: detail.UserID}
		default:
			continue
		}
		rows = append(rows, spec)
	}
	if detail.Working {
		rows = append(rows, modalRowSpec{Label: "Working..."})
	}
	return rows, headers
}

func windowMembersRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}

func memberRoleLabel(role domain.ChatMemberRole, tag string) string {
	label := ""
	switch role {
	case domain.ChatMemberRoleOwner:
		label = "Owner"
	case domain.ChatMemberRoleAdministrator:
		label = "Administrator"
	case domain.ChatMemberRoleRestricted:
		label = "Restricted"
	}
	tag = sanitizeDisplayString(tag)
	if label != "" && tag != "" {
		label += ": " + tag
	} else if tag != "" {
		label = tag
	}
	return label
}

func memberResultLabel(member domain.ChatMember) string {
	name := sanitizeDisplayString(member.User.Name)
	if name == "" {
		name = "Unknown"
	}
	parts := []string{name}
	if username := sanitizeDisplayString(member.User.Username); username != "" {
		parts = append(parts, "@"+strings.TrimPrefix(username, "@"))
	}
	if role := memberRoleLabel(member.Role, member.Tag); role != "" {
		parts = append(parts, role)
	}
	return strings.Join(parts, " · ")
}
