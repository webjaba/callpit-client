package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/webjaba/callpit-client/internal/config"
)

func TestSessionSetAccessToken(t *testing.T) {
	const (
		newToken = "1:new-secret"
	)
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "ok", input: "  " + newToken + "  ", want: newToken},
		{name: "empty key", input: "  ", want: testOldToken, wantErr: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := mustSetup(t)

			err := setup.SetAccessToken(tt.input)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Zero(t, setup.user)
				require.Equal(t, uint64(2), setup.generation)
			}
			cfg, readErr := config.Read(setup.configPath)
			require.NoError(t, readErr)
			require.Equal(t, tt.want, cfg.AccessToken)
		})
	}
}
