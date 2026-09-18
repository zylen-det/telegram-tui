package buildinfo

import "testing"

func TestCurrentUsesDevelopmentVersionByDefault(t *testing.T) {
	got := Current()
	if got.Name != "telegram-tui" {
		t.Fatalf("Name = %q, want telegram-tui", got.Name)
	}
	if got.Version != "dev" {
		t.Fatalf("Version = %q, want dev", got.Version)
	}
}
