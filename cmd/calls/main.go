package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/webjaba/callpit-client/internal/app"
	"github.com/webjaba/callpit-client/internal/config"
	"github.com/webjaba/callpit-client/internal/ui"
)

func main() {
	configPath, err := config.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := config.Load(configPath); err != nil && !errors.Is(err, config.ErrNotConfigured) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	session := app.NewSession(configPath)
	defer session.Close()
	if _, err := tea.NewProgram(ui.NewModel(session, configPath)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
