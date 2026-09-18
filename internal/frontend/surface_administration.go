package frontend

import (
	"image"
	"strconv"

	"github.com/zylen-det/telegram-tui/internal/app"
	"github.com/zylen-det/telegram-tui/internal/telegram"
	"github.com/zylen-det/telegram-tui/internal/ui"
)

func administrationFrame(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return image.Rectangle{}
	}
	return centeredSurfaceRectangle(bounds, min(72, bounds.Dx()), min(14, bounds.Dy())).Intersect(bounds)
}

func buildAdministrationLayer(model ui.ViewModel, styles renderStyles) surfaceResult {
	admin := model.Administration
	if admin == nil {
		return surfaceResult{Cursor: renderCursor{X: -1, Y: -1}}
	}
	bounds := image.Rect(0, 0, model.Width, model.Height)
	rows := []modalRowSpec{{Label: administrationIntro(admin), Header: true}}
	selectedRow := -1
	actionIndex := 0
	for index, item := range app.AdministrationMenuItems(admin) {
		if item.Header || item.Action.Action == app.NoAction {
			rows = append(rows, modalRowSpec{Label: sanitizeDisplayString(item.Label), Header: true})
			continue
		}
		row := modalRowSpec{
			ID:       "administration:row:" + strconv.Itoa(index),
			Label:    sanitizeDisplayString(item.Label),
			Selected: actionIndex == admin.Selected,
			Action:   item.Action,
		}
		if row.Selected {
			selectedRow = len(rows)
		}
		rows = append(rows, row)
		actionIndex++
	}
	switch {
	case admin.Working:
		rows = append(rows, modalRowSpec{Label: "Working..."})
	case admin.Loading || admin.MemberLoading:
		rows = append(rows, modalRowSpec{Label: "Loading administration..."})
	case admin.Error != nil:
		rows = append(rows, modalRowSpec{Label: sanitizeDisplayString(admin.Error.Message)})
	case actionIndex == 0:
		rows = append(rows, modalRowSpec{Label: "No management actions available"})
	}
	if selectedRow < 0 {
		selectedRow = 0
	}
	rows = windowAdministrationRows(rows, selectedRow, max(1, bounds.Dy()-5))
	title := administrationTitle(admin.Mode)
	if admin.Notice != "" {
		title += " · " + sanitizeDisplayString(admin.Notice)
	}
	return buildListModalWidth(bounds, title, rows, styles, administrationFrame(bounds).Dx())
}

func administrationTitle(mode app.AdministrationMode) string {
	switch mode {
	case app.AdministrationDefaultPermissions:
		return "Group permissions"
	case app.AdministrationMemberMenu:
		return "Manage member"
	case app.AdministrationAdminRightsEditor:
		return "Administrator rights"
	case app.AdministrationRestrictionsEditor:
		return "Member permissions"
	case app.AdministrationConfirmation:
		return "Confirm member action"
	default:
		return "Administration"
	}
}

func administrationIntro(admin *app.AdministrationState) string {
	if admin == nil {
		return ""
	}
	switch admin.Mode {
	case app.AdministrationDefaultPermissions:
		return "Allowed actions for ordinary members"
	case app.AdministrationMemberMenu:
		if admin.ReturnMembers != nil && admin.ReturnMembers.Detail != nil {
			return sanitizeDisplayString(admin.ReturnMembers.Detail.Name)
		}
		return "Member actions"
	case app.AdministrationAdminRightsEditor:
		return "Select rights for this administrator"
	case app.AdministrationRestrictionsEditor:
		return "Allowed actions for this member"
	case app.AdministrationConfirmation:
		return memberActionConsequence(admin.PendingAction)
	default:
		return ""
	}
}

func memberActionConsequence(action telegram.MemberAdministrationAction) string {
	switch action {
	case telegram.MemberAdministrationDemote:
		return "Remove administrator rights?"
	case telegram.MemberAdministrationUnrestrict:
		return "Remove member restrictions?"
	case telegram.MemberAdministrationRemove:
		return "Remove this member from the chat?"
	case telegram.MemberAdministrationBan:
		return "Ban this member from the chat?"
	default:
		return "Apply this member action?"
	}
}

func windowAdministrationRows(rows []modalRowSpec, selected, capacity int) []modalRowSpec {
	if capacity <= 0 || len(rows) <= capacity {
		return rows
	}
	start := max(0, selected-capacity+1)
	start = min(start, len(rows)-capacity)
	return rows[start : start+capacity]
}
