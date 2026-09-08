package ui

import (
	"path/filepath"
	"testing"

	"github.com/webjaba/callpit-client/internal/app"
)

type Setup struct {
	Model
}

func mustSetup(t *testing.T) Setup {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	return Setup{Model: NewModel(app.NewSession(path), path)}
}
