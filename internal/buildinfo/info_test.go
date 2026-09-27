package buildinfo

import "testing"

func TestCurrentUsesDevelopmentVersionByDefault(t *testing.T) {
	got := Current()
	if got.Name != "tuilegram" {
		t.Fatalf("Name = %q, want tuilegram", got.Name)
	}
	if got.Version != "dev" {
		t.Fatalf("Version = %q, want dev", got.Version)
	}
}
