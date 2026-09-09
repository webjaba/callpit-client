package audio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActivityDetector(t *testing.T) {
	var changes []bool
	detector := activityDetector{onChange: func(active bool) {
		changes = append(changes, active)
	}}
	loud := make([]int16, opusFrameSamples)
	for i := range loud {
		loud[i] = voiceThreshold + 1
	}
	silent := make([]int16, opusFrameSamples)

	detector.update(loud)
	detector.update(loud)
	for range voiceHangover/opusFrameSamples - 1 {
		detector.update(silent)
	}
	require.Equal(t, []bool{true}, changes)

	detector.update(silent)
	require.Equal(t, []bool{true, false}, changes)
}

func TestActivityDetectorStop(t *testing.T) {
	var changes []bool
	detector := activityDetector{onChange: func(active bool) {
		changes = append(changes, active)
	}}
	loud := make([]int16, opusFrameSamples)
	for i := range loud {
		loud[i] = voiceThreshold + 1
	}

	detector.update(loud)
	detector.stop()

	require.Equal(t, []bool{true, false}, changes)
}
