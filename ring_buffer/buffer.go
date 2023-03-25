package ring_buffer

type bufImpl struct {
	buffer []int16
	head   int
}

func NewRingBuffer(size int) *bufImpl {
	return &bufImpl{
		buffer: make([]int16, size),
		head:   0,
	}
}

func (r *bufImpl) Add(samples []int16) {
	for _, s := range samples {
		r.buffer[r.head] = s
		r.head = (r.head + 1) % len(r.buffer)
	}
}

func (r *bufImpl) Read() []int16 {
	samples := make([]int16, len(r.buffer))
	for i := 0; i < len(r.buffer); i++ {
		samples[i] = r.buffer[(r.head+i)%len(r.buffer)]
	}
	return samples
}

func (r *bufImpl) Clear() {
	for i := 0; i < len(r.buffer); i++ {
		r.buffer[i] = 0
	}
}
