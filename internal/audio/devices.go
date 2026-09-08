package audio

import (
	"fmt"

	"github.com/gen2brain/malgo"
)

type Device struct {
	ID        string
	Name      string
	IsDefault bool
}

func ListDevices() ([]Device, []Device, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize audio: %w", err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	inputs, err := devices(ctx.Context, malgo.Capture)
	if err != nil {
		return nil, nil, err
	}
	outputs, err := devices(ctx.Context, malgo.Playback)
	if err != nil {
		return nil, nil, err
	}
	return inputs, outputs, nil
}

func devices(ctx malgo.Context, kind malgo.DeviceType) ([]Device, error) {
	infos, err := ctx.Devices(kind)
	if err != nil {
		return nil, fmt.Errorf("list audio devices: %w", err)
	}

	result := make([]Device, 0, len(infos))
	for i := range infos {
		result = append(result, Device{
			ID:        infos[i].ID.String(),
			Name:      infos[i].Name(),
			IsDefault: infos[i].IsDefault != 0,
		})
	}
	return result, nil
}

func deviceID(ctx malgo.Context, kind malgo.DeviceType, id *string) (*malgo.DeviceID, error) {
	if id == nil {
		return nil, nil
	}

	infos, err := ctx.Devices(kind)
	if err != nil {
		return nil, fmt.Errorf("list audio devices: %w", err)
	}
	for i := range infos {
		if infos[i].ID.String() == *id {
			return &infos[i].ID, nil
		}
	}

	return nil, fmt.Errorf("audio device is no longer available")
}
