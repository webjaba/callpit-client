package audio

import "sync"

type byteBuffer struct {
	mu    sync.Mutex
	data  []byte
	read  int
	count int
}

func newByteBuffer(size int) *byteBuffer {
	return &byteBuffer{data: make([]byte, size)}
}

func (b *byteBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, value := range data {
		if b.count == len(b.data) {
			b.read = (b.read + 1) % len(b.data)
			b.count--
		}
		write := (b.read + b.count) % len(b.data)
		b.data[write] = value
		b.count++
	}
}

func (b *byteBuffer) ReadFull(dst []byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count < len(dst) {
		return false
	}
	for i := range dst {
		dst[i] = b.data[b.read]
		b.read = (b.read + 1) % len(b.data)
	}
	b.count -= len(dst)
	return true
}

type sampleBuffer struct {
	mu      sync.Mutex
	samples []int16
	read    int
	count   int
}

func newSampleBuffer(size int) *sampleBuffer {
	return &sampleBuffer{samples: make([]int16, size)}
}

func (b *sampleBuffer) Write(samples []int16) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, sample := range samples {
		if b.count == len(b.samples) {
			b.read = (b.read + 1) % len(b.samples)
			b.count--
		}
		write := (b.read + b.count) % len(b.samples)
		b.samples[write] = sample
		b.count++
	}
}

func (b *sampleBuffer) MixInto(dst []int32) {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := min(len(dst), b.count)
	for i := range count {
		dst[i] += int32(b.samples[b.read])
		b.read = (b.read + 1) % len(b.samples)
	}
	b.count -= count
}
