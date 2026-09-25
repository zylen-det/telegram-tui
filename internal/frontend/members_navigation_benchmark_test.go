package frontend

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

// BenchmarkMembersLongPress measures the complete update/render loop for a
// populated members modal, including the selector synchronization and frame.
func BenchmarkMembersLongPress(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("members=%d", count), func(b *testing.B) {
			state := membersBaseState(domain.ChatSupergroup)
			state.Width, state.Height = 100, 30
			state.Focus = FocusMembers
			state.Members = &MembersState{RequestID: 10, ChatID: 9, Done: true}
			for i := 1; i <= count; i++ {
				state.Members.Results = append(state.Members.Results, domain.ChatMember{User: domain.User{ID: domain.UserID(i), Name: fmt.Sprintf("Member %d", i)}})
			}
			model, err := NewAppModel(state, NewHandler(context.Background(), nil, nil, nil, nil))
			if err != nil {
				b.Fatal(err)
			}
			model.syncListModalController()
			key := tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
			for _, phase := range []string{"update+view", "update", "view"} {
				b.Run(phase, func(b *testing.B) {
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if phase != "view" {
							next, _ := model.Update(key)
							model = next.(AppModel)
						}
						if phase != "update" {
							_ = model.View()
						}
					}
				})
			}
		})
	}
}
