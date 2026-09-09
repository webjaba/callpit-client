package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/pion/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	sampleRate       = 48000
	opusFrameSamples = 960
	remoteBufferSize = sampleRate / 2
	voiceThreshold   = 700
	voiceHangover    = sampleRate * 300 / 1000
)

type activityDetector struct {
	active        bool
	silentSamples int
	onChange      func(bool)
}

func (d *activityDetector) update(samples []int16) {
	var energy int64
	for _, sample := range samples {
		value := int64(sample)
		energy += value * value
	}
	d.updateEnergy(energy, len(samples))
}

func (d *activityDetector) updateBytes(data []byte) {
	var energy int64
	for i := 0; i+1 < len(data); i += 2 {
		value := int64(int16(binary.LittleEndian.Uint16(data[i:])))
		energy += value * value
	}
	d.updateEnergy(energy, len(data)/2)
}

func (d *activityDetector) updateEnergy(energy int64, samples int) {
	speaking := samples > 0 && energy >= int64(voiceThreshold*voiceThreshold*samples)
	if speaking {
		d.silentSamples = 0
		if !d.active {
			d.active = true
			d.onChange(true)
		}
		return
	}
	if !d.active {
		return
	}
	d.silentSamples += samples
	if d.silentSamples >= voiceHangover {
		d.active = false
		d.silentSamples = 0
		d.onChange(false)
	}
}

func (d *activityDetector) stop() {
	if d.active {
		d.active = false
		d.onChange(false)
	}
}

type Engine struct {
	ctx        *malgo.AllocatedContext
	track      *webrtc.TrackLocalStaticSample
	capture    *malgo.Device
	playback   *malgo.Device
	input      *byteBuffer
	inputReady chan struct{}
	cancel     context.CancelFunc
	muted      atomic.Bool
	deafened   atomic.Bool
	mu         sync.Mutex
	remotes    map[string]*sampleBuffer
	remoteCtx  map[string]context.CancelFunc
	scratch    []int32
	activity   func(string, bool)
	localVoice activityDetector
}

func New(track *webrtc.TrackLocalStaticSample, onActivity func(string, bool)) (*Engine, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize audio: %w", err)
	}

	if onActivity == nil {
		onActivity = func(string, bool) {}
	}
	engine := &Engine{
		ctx:        ctx,
		track:      track,
		input:      newByteBuffer(opusFrameSamples * 2 * 16),
		inputReady: make(chan struct{}, 1),
		remotes:    make(map[string]*sampleBuffer),
		remoteCtx:  make(map[string]context.CancelFunc),
		activity:   onActivity,
	}
	engine.localVoice.onChange = func(active bool) { onActivity("", active) }
	return engine, nil
}

func (e *Engine) Devices() ([]Device, []Device, error) {
	inputs, err := devices(e.ctx.Context, malgo.Capture)
	if err != nil {
		return nil, nil, err
	}
	outputs, err := devices(e.ctx.Context, malgo.Playback)
	if err != nil {
		return nil, nil, err
	}
	return inputs, outputs, nil
}

func (e *Engine) Start(inputID, outputID *string) error {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	go e.encode(ctx)

	if err := e.SetOutput(outputID); err != nil {
		cancel()
		return err
	}
	if err := e.SetInput(inputID); err != nil {
		e.playback.Uninit()
		e.playback = nil
		cancel()
		return err
	}

	return nil
}

func (e *Engine) SetInput(id *string) error {
	selected, err := deviceID(e.ctx.Context, malgo.Capture, id)
	if err != nil {
		return err
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = sampleRate
	cfg.Alsa.NoMMap = 1
	if selected != nil {
		cfg.Capture.DeviceID = selected.Pointer()
	}

	device, err := malgo.InitDevice(e.ctx.Context, cfg, malgo.DeviceCallbacks{Data: e.captureFrames})
	if err != nil {
		return fmt.Errorf("open microphone: %w", err)
	}
	if err := device.Start(); err != nil {
		device.Uninit()
		return fmt.Errorf("start microphone: %w", err)
	}

	e.mu.Lock()
	old := e.capture
	e.capture = device
	e.mu.Unlock()
	if old != nil {
		old.Uninit()
	}
	return nil
}

func (e *Engine) SetOutput(id *string) error {
	selected, err := deviceID(e.ctx.Context, malgo.Playback, id)
	if err != nil {
		return err
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = 2
	cfg.SampleRate = sampleRate
	cfg.Alsa.NoMMap = 1
	if selected != nil {
		cfg.Playback.DeviceID = selected.Pointer()
	}

	device, err := malgo.InitDevice(e.ctx.Context, cfg, malgo.DeviceCallbacks{Data: e.playbackFrames})
	if err != nil {
		return fmt.Errorf("open output device: %w", err)
	}
	if err := device.Start(); err != nil {
		device.Uninit()
		return fmt.Errorf("start output device: %w", err)
	}

	e.mu.Lock()
	old := e.playback
	e.playback = device
	e.mu.Unlock()
	if old != nil {
		old.Uninit()
	}
	return nil
}

func (e *Engine) SetMuted(value bool) {
	e.muted.Store(value)
}

func (e *Engine) SetDeafened(value bool) {
	e.deafened.Store(value)
}

func (e *Engine) AddRemote(peerID string, track *webrtc.TrackRemote) {
	e.RemoveRemote(peerID)

	ctx, cancel := context.WithCancel(context.Background())
	buffer := newSampleBuffer(remoteBufferSize)
	e.mu.Lock()
	e.remotes[peerID] = buffer
	e.remoteCtx[peerID] = cancel
	e.mu.Unlock()

	go e.decode(ctx, peerID, track, buffer)
}

func (e *Engine) RemoveRemote(peerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cancel := e.remoteCtx[peerID]; cancel != nil {
		cancel()
	}
	delete(e.remoteCtx, peerID)
	delete(e.remotes, peerID)
}

func (e *Engine) Close() error {
	if e.cancel != nil {
		e.cancel()
	}

	e.mu.Lock()
	for _, cancel := range e.remoteCtx {
		cancel()
	}
	e.remoteCtx = make(map[string]context.CancelFunc)
	e.remotes = make(map[string]*sampleBuffer)
	capture := e.capture
	playback := e.playback
	e.capture = nil
	e.playback = nil
	e.mu.Unlock()
	if capture != nil {
		capture.Uninit()
	}
	if playback != nil {
		playback.Uninit()
	}

	err := e.ctx.Uninit()
	e.ctx.Free()
	return err
}

func (e *Engine) captureFrames(_ []byte, input []byte, _ uint32) {
	e.input.Write(input)
	select {
	case e.inputReady <- struct{}{}:
	default:
	}
}

func (e *Engine) playbackFrames(output, _ []byte, frameCount uint32) {
	clear(output)

	frames := int(frameCount)
	e.mu.Lock()
	if len(e.scratch) < frames {
		e.scratch = make([]int32, frames)
	}
	mix := e.scratch[:frames]
	clear(mix)
	for _, buffer := range e.remotes {
		buffer.MixInto(mix)
	}
	e.mu.Unlock()
	if e.deafened.Load() {
		return
	}

	for i, sample := range mix {
		if sample > 32767 {
			sample = 32767
		} else if sample < -32768 {
			sample = -32768
		}
		value := uint16(int16(sample))
		binary.LittleEndian.PutUint16(output[i*4:], value)
		binary.LittleEndian.PutUint16(output[i*4+2:], value)
	}
}

func (e *Engine) encode(ctx context.Context) {
	encoder, err := opus.NewEncoder(
		opus.WithSampleRate(sampleRate),
		opus.WithChannels(1),
		opus.WithApplication(opus.ApplicationVoIP),
		opus.WithBitrate(32000),
	)
	if err != nil {
		return
	}

	pcm := make([]byte, opusFrameSamples*2)
	packet := make([]byte, 1275)
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.inputReady:
			for e.input.ReadFull(pcm) {
				if e.muted.Load() {
					clear(pcm)
				}
				e.localVoice.updateBytes(pcm)
				n, err := encoder.Encode(pcm, packet)
				if err == nil {
					_ = e.track.WriteSample(media.Sample{Data: append([]byte(nil), packet[:n]...), Duration: 20 * time.Millisecond})
				}
			}
		}
	}
}

func (e *Engine) decode(ctx context.Context, peerID string, track *webrtc.TrackRemote, buffer *sampleBuffer) {
	decoder, err := opus.NewDecoderWithOutput(sampleRate, 1)
	if err != nil {
		return
	}
	pcm := make([]int16, sampleRate*120/1000)
	detector := activityDetector{onChange: func(active bool) { e.activity(peerID, active) }}
	defer detector.stop()

	for {
		packet, _, err := track.ReadRTP()
		if errors.Is(err, io.EOF) || ctx.Err() != nil {
			return
		}
		if err != nil {
			return
		}
		n, err := decoder.DecodeToInt16(packet.Payload, pcm)
		if err != nil {
			continue
		}
		detector.update(pcm[:n])
		buffer.Write(pcm[:n])
	}
}
