package native

import (
	"fmt"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/capabilities"
)

// io_mmap.go — memory-mapped files. IO.mmap maps a file's bytes into a
// Mmap resource; `read region {offset length enc}` COPIES bytes out (a
// read must never hand AQL a slice that a later close unmaps — a
// use-after-free), `write region bytes {offset}` splices INTO the mapping
// in place, IO.flush syncs, and the polymorphic IO.close unmaps.

// MmapInfo is the payload behind a Mmap handle: a live mapped region,
// closed exactly once.
type MmapInfo struct {
	ID       string
	Path     string
	Writable bool
	region   capabilities.MmapRegion
	once     sync.Once
	closeErr error
}

func (mi *MmapInfo) Close() error {
	mi.once.Do(func() { mi.closeErr = mi.region.Close() })
	return mi.closeErr
}

// mmapFormatBehavior renders a Mmap as "Mmap(id,path)".
type mmapFormatBehavior struct{}

func (mmapFormatBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (mmapFormatBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (mmapFormatBehavior) Format(v Value) string {
	if mi, ok := asMmapInfo(v); ok {
		return fmt.Sprintf("Mmap(%s,%s)", mi.ID, mi.Path)
	}
	return "Mmap(nil)"
}

// MintMmapType / NewMmapType mint the module-scoped Mmap resource type.
func MintMmapType(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("Mmap", eng.TIdeal, mmapFormatBehavior{})
}
func NewMmapType() *Type {
	return eng.NewDynamicTypeTable().MintTypeWithBehavior("Mmap", eng.TIdeal, mmapFormatBehavior{})
}

// asMmapInfo unwraps a Mmap handle's payload.
func asMmapInfo(v Value) (*MmapInfo, bool) {
	ext, ok := v.Data.(ExtensionPayload)
	if !ok {
		return nil, false
	}
	mi, ok := ext.Body.(*MmapInfo)
	return mi, ok
}

// doMmapWord implements IO.mmap p {offset length writable}: map a file
// region. writable permits in-place writes flushed on flush/close.
func doMmapWord(args []Value, r *Registry, mmapType *Type) ([]Value, error) {
	path := extractPath(args[0])
	var offset int64
	length := 0
	writable := false
	if len(args) > 1 {
		if o, ok := mapIntOpt(args[1], "offset"); ok {
			offset = o
		}
		if l, ok := mapIntOpt(args[1], "length"); ok {
			length = int(l)
		}
		writable = mapBoolOpt(args[1], "writable", false)
	}
	r.NoteEffect()
	region, err := EffectiveFileOps(r).Mmap(path, offset, length, writable)
	if err != nil {
		return nil, r.AqlError("mmap_error", fmt.Sprintf("mmap: %v", err), "mmap")
	}
	info := &MmapInfo{ID: GenerateID("M_"), Path: path, Writable: writable, region: region}
	return []Value{eng.NewValueRaw(mmapType, ExtensionPayload{Body: info})}, nil
}

// readMmapWord reads from a mapped region: {offset}/{length} window it,
// {enc} decodes ('bytes' returns Bytes, else text). The bytes are COPIED
// out of the mapping so a later close (munmap) can never invalidate the
// AQL value.
func readMmapWord(args []Value, r *Registry) ([]Value, error) {
	mi, ok := asMmapInfo(args[0])
	if !ok {
		return nil, r.AqlError("read_error", fmt.Sprintf("read: not a Mmap region (got %s)", args[0].Parent), "read")
	}
	buf := mi.region.Bytes()
	enc := "utf8"
	offset, length := 0, -1
	if len(args) > 1 {
		if e, ok := mapStrOpt(args[1], "enc"); ok {
			enc = e
		}
		if o, ok := mapIntOpt(args[1], "offset"); ok {
			offset = int(o)
		}
		if l, ok := mapIntOpt(args[1], "length"); ok {
			length = int(l)
		}
	}
	lo, hi, werr := regionSlice(offset, length, len(buf))
	if werr != nil {
		return nil, r.AqlError("read_error", fmt.Sprintf("read: %v", werr), "read")
	}
	out := make([]byte, hi-lo) // COPY out of the mapping (munmap-safe)
	copy(out, buf[lo:hi])
	if enc == "bytes" || enc == "binary" {
		return []Value{NewBytesValue(out)}, nil
	}
	text, derr := decodeEnc(out, enc)
	if derr != nil {
		return nil, r.AqlError("read_error", fmt.Sprintf("read: %v", derr), "read")
	}
	return []Value{NewString(text)}, nil
}

// writeMmapWord splices bytes INTO the mapping at {offset} (default 0),
// in place — an mmap cannot grow, so the write must fit the region. It
// returns the region for threading. A read-only region refuses.
func writeMmapWord(args []Value, r *Registry) ([]Value, error) {
	mi, ok := asMmapInfo(args[0])
	if !ok {
		return nil, r.AqlError("write_error", fmt.Sprintf("write: not a Mmap region (got %s)", args[0].Parent), "write")
	}
	if !mi.Writable {
		return nil, r.AqlError("write_error", "write: this Mmap region is read-only", "write")
	}
	data, err := handleWriteBytes(args[1])
	if err != nil {
		return nil, r.AqlError("write_error", fmt.Sprintf("write: %v", err), "write")
	}
	offset := 0
	if len(args) > 2 {
		if o, ok := mapIntOpt(args[2], "offset"); ok {
			offset = int(o)
		}
	}
	buf := mi.region.Bytes()
	if offset < 0 || offset+len(data) > len(buf) {
		return nil, r.AqlError("write_error", "write: does not fit the mapped region (an mmap cannot grow)", "write")
	}
	r.NoteEffect()
	copy(buf[offset:], data)
	return []Value{args[0]}, nil
}

// regionSlice validates an {offset,length} window against a region of
// size n (length<0 means "to end").
func regionSlice(offset, length, n int) (lo, hi int, err error) {
	if offset < 0 || offset > n {
		return 0, 0, fmt.Errorf("offset %d out of range [0,%d]", offset, n)
	}
	hi = n
	if length >= 0 {
		hi = offset + length
	}
	if hi > n {
		return 0, 0, fmt.Errorf("window [%d,%d) exceeds region size %d", offset, hi, n)
	}
	return offset, hi, nil
}
