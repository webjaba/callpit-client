package signaling

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRoom(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "ok", input: "friends"},
		{name: "empty room", wantErr: true},
		{name: "surrounding spaces", input: " friends ", wantErr: true},
		{name: "slash", input: "friends/evening", wantErr: true},
		{name: "control character", input: "friends\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = mustSetup(t)

			err := validateRoom(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
