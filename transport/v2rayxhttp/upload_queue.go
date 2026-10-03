package xhttp

import (
	"container/heap"
	"io"
	"sync/atomic"

	"github.com/sagernet/sing-box/common/xray/signal/done"
	E "github.com/sagernet/sing/common/exceptions"
)

type Packet struct {
	Reader  *httpServerConn
	Payload []byte
	Seq     uint64
}

type uploadQueue struct {
	reader        atomic.Pointer[httpServerConn]
	pushedPackets chan Packet
	heap          uploadHeap
	nextSeq       uint64
	maxPackets    int
	closed        *done.Instance
}

func NewUploadQueue(maxPackets int) *uploadQueue {
	return &uploadQueue{
		pushedPackets: make(chan Packet, maxPackets),
		heap:          uploadHeap{},
		nextSeq:       0,
		closed:        done.New(),
		maxPackets:    maxPackets,
	}
}

func (h *uploadQueue) Push(p Packet) error {
	if h.reader.Load() != nil || (p.Reader != nil && !h.reader.CompareAndSwap(nil, p.Reader)) {
		return E.New("h.reader already exists")
	}
	select {
	case h.pushedPackets <- p:
		if h.closed.Done() {
			return E.New("packet queue closed")
		}
		return nil
	case <-h.closed.Wait():
		return E.New("packet queue closed")
	}
}

func (h *uploadQueue) Close() error {
	h.closed.Close()
	if reader := h.reader.Load(); reader != nil {
		return reader.Close()
	}
	return nil
}

func (h *uploadQueue) Read(b []byte) (int, error) {
	if reader := h.reader.Load(); reader != nil {
		return reader.Read(b)
	}
	if h.closed.Done() {
		return 0, io.EOF
	}
	if len(h.heap) == 0 {
		select {
		case p := <-h.pushedPackets:
			if p.Reader != nil {
				return p.Reader.Read(b)
			}
			heap.Push(&h.heap, p)
		case <-h.closed.Wait():
			return 0, io.EOF
		}
	}
	for len(h.heap) > 0 {
		packet := heap.Pop(&h.heap).(Packet)
		if packet.Seq == h.nextSeq {
			n := min(len(b), len(packet.Payload))
			copy(b, packet.Payload)
			if n < len(packet.Payload) {
				packet.Payload = packet.Payload[n:]
				heap.Push(&h.heap, packet)
			} else {
				h.nextSeq = packet.Seq + 1
			}
			return n, nil
		}
		if packet.Seq > h.nextSeq {
			if len(h.heap) > h.maxPackets {
				return 0, E.New("packet queue is too large")
			}
			heap.Push(&h.heap, packet)
			select {
			case p := <-h.pushedPackets:
				heap.Push(&h.heap, p)
			case <-h.closed.Wait():
				return 0, io.EOF
			}
		}
	}
	return 0, nil
}

type uploadHeap []Packet

func (h uploadHeap) Len() int           { return len(h) }
func (h uploadHeap) Less(i, j int) bool { return h[i].Seq < h[j].Seq }
func (h uploadHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *uploadHeap) Push(x any)        { *h = append(*h, x.(Packet)) }
func (h *uploadHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
