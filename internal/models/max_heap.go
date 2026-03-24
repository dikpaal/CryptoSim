package models

type MaxHeap []*Order

func (h MaxHeap) Len() int {
	return len(h)
}

func (h MaxHeap) Less(i, j int) bool {
	if h[i].Price != h[j].Price {
		return h[i].Price > h[j].Price
	}
	return h[i].CreatedAt.Before(h[j].CreatedAt)
}

func (h MaxHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *MaxHeap) Push(x interface{}) {
	*h = append(*h, x.(*Order))
}

func (h *MaxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}
