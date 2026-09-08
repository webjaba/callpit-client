package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/webjaba/callpit-client/internal/config"
)

func TestLoad(t *testing.T) {
	const valid = `{
  "access_token": "1:secret",
  "input_device_id": null,
  "output_device_id": null
}
`
	validInput := valid
	invalidJSON := "{"
	tests := []struct {
		name    string
		input   *string
		want    config.Config
		wantErr error
	}{
		{
			name:  "ok",
			input: &validInput,
			want:  config.Config{AccessToken: "1:secret"},
		},
		{name: "missing file", wantErr: config.ErrNotConfigured},
		{
			name:    "invalid JSON",
			input:   &invalidJSON,
			wantErr: errors.New("decode config"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = mustSetup(t)
			path := filepath.Join(t.TempDir(), "calls", "config.json")
			if tt.input != nil {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(*tt.input), 0o644))
			}

			got, err := config.Load(path)

			if tt.wantErr != nil {
				if errors.Is(tt.wantErr, config.ErrNotConfigured) {
					require.ErrorIs(t, err, tt.wantErr)
					require.FileExists(t, path)
					return
				}
				require.ErrorContains(t, err, tt.wantErr.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		})
	}
}

func TestRead(t *testing.T) {
	const input = `{
  "access_token": "",
  "input_device_id": null,
  "output_device_id": null
}
`
	tests := []struct {
		name  string
		input string
		want  config.Config
	}{
		{
			name:  "ok",
			input: input,
			want:  config.Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = mustSetup(t)
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.input), 0o600))

			got, err := config.Read(path)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSave(t *testing.T) {
	inputID := "microphone-id"
	tests := []struct {
		name  string
		input config.Config
	}{
		{
			name: "ok",
			input: config.Config{
				AccessToken:   "1:secret",
				InputDeviceID: &inputID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = mustSetup(t)
			path := filepath.Join(t.TempDir(), "calls", "config.json")

			err := config.Save(path, tt.input)

			require.NoError(t, err)
			got, err := config.Load(path)
			require.NoError(t, err)
			require.Equal(t, tt.input, got)
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   config.Config
		wantErr string
	}{
		{name: "ok", input: config.Config{AccessToken: "1:secret"}},
		{name: "missing token", input: config.Config{}, wantErr: config.ErrNotConfigured.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = mustSetup(t)

			err := tt.input.Validate()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
