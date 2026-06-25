package debugserve

import "runtime"

// HeapStats is the JSON shape of /debug/heap.
type HeapStats struct {
	Alloc       uint64 `json:"alloc"`
	TotalAlloc  uint64 `json:"total_alloc"`
	HeapObjects uint64 `json:"heap_objects"`
	NumGC       uint32 `json:"num_gc"`
}

// ReadHeap snapshots the Go runtime heap stats.
func ReadHeap() HeapStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return HeapStats{
		Alloc:       m.Alloc,
		TotalAlloc:  m.TotalAlloc,
		HeapObjects: m.HeapObjects,
		NumGC:       m.NumGC,
	}
}
