package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/webjaba/callpit-client/internal/api"
	"github.com/webjaba/callpit-client/internal/config"
)

const testOldToken = "1:old-secret"

type Setup struct {
	*Session
}

func mustSetup(t *testing.T) Setup {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, config.Save(path, config.Config{
		AccessToken: testOldToken,
	}))
	session := NewSession(path)
	session.generation = 1
	session.user = api.User{ID: 1, Username: "alex"}
	return Setup{Session: session}
}
