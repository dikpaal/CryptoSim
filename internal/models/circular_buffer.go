package models

import "sync"

type CircularBuffer struct {
	buffer      []Trade
	writeIdx    int
	currentSize int
	maxCapacity int
	mu          sync.RWMutex
}

func NewCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{
		buffer:      make([]Trade, capacity),
		writeIdx:    0,
		currentSize: 0,
		maxCapacity: capacity,
	}
}

func (circularBuffer *CircularBuffer) Add(trade Trade) {
	circularBuffer.mu.Lock()
	defer circularBuffer.mu.Unlock()

	circularBuffer.buffer[circularBuffer.writeIdx] = trade
	circularBuffer.writeIdx = (circularBuffer.writeIdx + 1) % circularBuffer.maxCapacity

	if circularBuffer.currentSize < circularBuffer.maxCapacity {
		circularBuffer.currentSize++
	}

}

func (circularBuffer *CircularBuffer) FlushAll() []Trade {
	circularBuffer.mu.Lock()
	defer circularBuffer.mu.Unlock()

	result := make([]Trade, circularBuffer.currentSize)
	copy(result, circularBuffer.buffer[:circularBuffer.currentSize])

	circularBuffer.writeIdx = 0
	circularBuffer.currentSize = 0

	return result
}

func (circularBuffer *CircularBuffer) Len() int {
	circularBuffer.mu.RLock()
	defer circularBuffer.mu.RUnlock()
	return circularBuffer.currentSize
}

func (circularBuffer *CircularBuffer) IsFull() bool {
	circularBuffer.mu.RLock()
	defer circularBuffer.mu.RUnlock()
	return circularBuffer.currentSize == circularBuffer.maxCapacity
}
