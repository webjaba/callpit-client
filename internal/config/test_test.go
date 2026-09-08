package config_test

import (
	"testing"

	"github.com/webjaba/callpit-client/internal/config"
)

type Setup struct {
	*config.Config
}

func mustSetup(t *testing.T) Setup {
	t.Helper()
	return Setup{Config: &config.Config{}}
}
