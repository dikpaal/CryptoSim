package models

import "sync"

type CircularBufferSnapshot struct {
	buffer      []OrderbookSnapshot
	writeIdx    int
	currentSize int
	maxCapacity int
	mu          sync.RWMutex
}

func NewCircularBufferSnapshot(capacity int) *CircularBufferSnapshot {
	return &CircularBufferSnapshot{
		buffer:      make([]OrderbookSnapshot, capacity),
		writeIdx:    0,
		currentSize: 0,
		maxCapacity: capacity,
	}
}

func (cb *CircularBufferSnapshot) Add(snapshot OrderbookSnapshot) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.buffer[cb.writeIdx] = snapshot
	cb.writeIdx = (cb.writeIdx + 1) % cb.maxCapacity

	if cb.currentSize < cb.maxCapacity {
		cb.currentSize++
	}
}

func (cb *CircularBufferSnapshot) FlushAll() []OrderbookSnapshot {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	result := make([]OrderbookSnapshot, cb.currentSize)
	copy(result, cb.buffer[:cb.currentSize])

	cb.writeIdx = 0
	cb.currentSize = 0

	return result
}

func (cb *CircularBufferSnapshot) Len() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentSize
}

func (cb *CircularBufferSnapshot) IsFull() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentSize == cb.maxCapacity
}
