package smux

type shaperHeap []writeRequest

func (h shaperHeap) Len() int            { return len(h) }
func (h shaperHeap) Less(i, j int) bool  { return h[i].prio < h[j].prio }
func (h shaperHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *shaperHeap) Push(x interface{}) { *h = append(*h, x.(writeRequest)) }

func (h *shaperHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
