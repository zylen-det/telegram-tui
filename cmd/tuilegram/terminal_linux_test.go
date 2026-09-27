package main

import (
	"os"
	"strings"
	"testing"
)

func TestProductionBubbleTeaOwnsTerminalLifecycle(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"tea.NewProgram(",
		"tea.WithContext(ctx)",
		"tea.WithInput(options.stdin)",
		"tea.WithOutput(output)",
		"tea.WithoutSignalHandler()",
		"frontend.NewAppModel(",
		"kitty.NewManager(options.stdout)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("production entrypoint missing %q", required)
		}
	}
	if strings.Count(text, "signal.Notify(") != 1 || strings.Count(text, "signal.Stop(") != 1 {
		t.Error("production entrypoint must scope exactly one signal subscription")
	}
}

func TestProductionNoLegacyTTY(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, obsolete := range []string{
		"gotui",
		"terminalTTY",
		"terminalHandle",
		"productionSuspend",
		"runComponents",
		"term.MakeRaw",
	} {
		if strings.Contains(text, obsolete) {
			t.Errorf("production entrypoint retains legacy terminal owner %q", obsolete)
		}
	}
}
