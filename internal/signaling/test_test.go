package signaling

import "testing"

type Setup struct {
	*Client
}

func mustSetup(t *testing.T) Setup {
	t.Helper()
	return Setup{}
}
