package server

import (
	"sync"
)

type RingBuffer struct {
	buf     []byte
	size    int
	maxSize int
	head    int
	mu      sync.RWMutex
}

func NewRingBuffer(maxSize int) *RingBuffer {
	if maxSize <= 0 {
		maxSize = 512 * 1024 // Default 512KB
	}
	return &RingBuffer{
		buf:     make([]byte, maxSize),
		maxSize: maxSize,
	}
}

func (r *RingBuffer) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n = len(p)
	if n == 0 {
		return 0, nil
	}

	for _, b := range p {
		r.buf[r.head] = b
		r.head = (r.head + 1) % r.maxSize
		if r.size < r.maxSize {
			r.size++
		}
	}
	return n, nil
}

func (r *RingBuffer) Bytes() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return []byte{}
	}

	result := make([]byte, r.size)
	if r.size < r.maxSize {
		copy(result, r.buf[:r.size])
	} else {
		// Circular copy
		tail := r.head
		copy(result, r.buf[tail:])
		copy(result[r.maxSize-tail:], r.buf[:tail])
	}
	return result
}
