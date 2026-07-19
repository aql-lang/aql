package tuikit

import (
	"errors"
	"strings"
	"testing"
)

// --- Frame ---

func TestNewFrameClampsNegative(t *testing.T) {
	f := NewFrame(-3, -1)
	if f.Cols != 0 || f.Rows != 0 || len(f.Cells) != 0 {
		t.Fatalf("negative dims not clamped: %+v", f)
	}
}

func TestFrameAtSetBounds(t *testing.T) {
	f := NewFrame(2, 2)
	if ok := f.Set(1, 1, Cell{Content: "x", Width: 1}); !ok {
		t.Fatal("in-bounds Set reported false")
	}
	c, ok := f.At(1, 1)
	if !ok || c.Content != "x" {
		t.Fatalf("At(1,1) = %+v, %v", c, ok)
	}
	// every out-of-bounds edge, both accessors
	for _, xy := range [][2]int{{-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
		if _, ok := f.At(xy[0], xy[1]); ok {
			t.Errorf("At%v in bounds", xy)
		}
		if ok := f.Set(xy[0], xy[1], Cell{}); ok {
			t.Errorf("Set%v in bounds", xy)
		}
	}
}

func TestFrameCloneIsIndependent(t *testing.T) {
	f := NewFrame(1, 1)
	f.Set(0, 0, Cell{Content: "a"})
	c := f.Clone()
	f.Set(0, 0, Cell{Content: "b"})
	got, _ := c.At(0, 0)
	if got.Content != "a" {
		t.Fatalf("clone mutated: %q", got.Content)
	}
}

func TestRenderRowsBlanksAndContent(t *testing.T) {
	f := NewFrame(3, 2)
	f.Set(0, 0, Cell{Content: "h"})
	f.Set(1, 0, Cell{Content: "i"})
	rows := renderRows(f)
	if rows[0] != "hi " || rows[1] != "   " {
		t.Fatalf("renderRows = %q", rows)
	}
}

// --- VirtualBackend lifecycle ---

func TestVirtualOpenCloseLifecycle(t *testing.T) {
	vb := NewVirtualBackend(4, 2)
	info, err := vb.Open(OpenOpts{Title: "boot"})
	if err != nil || info.Cols != 4 || info.Rows != 2 {
		t.Fatalf("Open = %+v, %v", info, err)
	}
	if vb.Title() != "boot" {
		t.Fatalf("Open did not apply opts.Title: %q", vb.Title())
	}
	if _, err := vb.Open(OpenOpts{}); err == nil {
		t.Fatal("second Open succeeded")
	}
	if err := vb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := vb.Close(); err != nil {
		t.Fatalf("second Close not idempotent: %v", err)
	}
	if _, err := vb.Open(OpenOpts{}); err == nil {
		t.Fatal("Open after Close succeeded")
	}
}

func TestVirtualOpenErrKnob(t *testing.T) {
	vb := NewVirtualBackend(1, 1)
	vb.OpenErr = errors.New("boom")
	if _, err := vb.Open(OpenOpts{}); err == nil {
		t.Fatal("OpenErr knob ignored")
	}
}

func TestVirtualCloseErrKnob(t *testing.T) {
	vb := NewVirtualBackend(1, 1)
	vb.CloseErr = errors.New("boom")
	if err := vb.Close(); err == nil {
		t.Fatal("CloseErr knob ignored")
	}
	vb.CloseErr = nil
	if err := vb.Close(); err != nil {
		t.Fatalf("Close after clearing knob: %v", err)
	}
	if err := vb.Close(); err != nil {
		t.Fatalf("idempotent Close: %v", err)
	}
}

// --- events ---

func TestVirtualInjectAndEvents(t *testing.T) {
	vb := NewVirtualBackend(1, 1)
	vb.Inject(Event{Tag: "key", Key: "a", Char: "a"})
	ev := <-vb.Events()
	if ev.Key != "a" {
		t.Fatalf("event = %+v", ev)
	}
	if err := vb.Close(); err != nil {
		t.Fatal(err)
	}
	// after Close the channel is closed and Inject is a no-op
	vb.Inject(Event{Tag: "key", Key: "b"})
	if _, ok := <-vb.Events(); ok {
		t.Fatal("event delivered after Close")
	}
}

func TestVirtualInjectDropsWhenFull(t *testing.T) {
	vb := NewVirtualBackend(1, 1)
	for i := 0; i < virtualEventBuffer+5; i++ {
		vb.Inject(Event{Tag: "key", Key: "x"})
	}
	// buffer holds exactly virtualEventBuffer; the overflow was dropped
	n := 0
	for {
		select {
		case <-vb.Events():
			n++
			continue
		default:
		}
		break
	}
	if n != virtualEventBuffer {
		t.Fatalf("buffered %d events, want %d", n, virtualEventBuffer)
	}
}

// --- size/title/bell knobs ---

func TestVirtualSizeTitleBell(t *testing.T) {
	vb := NewVirtualBackend(7, 3)
	c, r, err := vb.Size()
	if err != nil || c != 7 || r != 3 {
		t.Fatalf("Size = %d,%d,%v", c, r, err)
	}
	vb.SizeErr = errors.New("nope")
	if _, _, err := vb.Size(); err == nil {
		t.Fatal("SizeErr knob ignored")
	}
	if err := vb.SetTitle("hi"); err != nil || vb.Title() != "hi" {
		t.Fatalf("SetTitle/Title = %v/%q", err, vb.Title())
	}
	vb.TitleErr = errors.New("nope")
	if err := vb.SetTitle("x"); err == nil {
		t.Fatal("TitleErr knob ignored")
	}
	if err := vb.Bell(); err != nil || vb.Bells() != 1 {
		t.Fatalf("Bell/Bells = %v/%d", err, vb.Bells())
	}
	vb.BellErr = errors.New("nope")
	if err := vb.Bell(); err == nil {
		t.Fatal("BellErr knob ignored")
	}
}

// --- present + history ---

func TestVirtualPresentHistoryAndScreens(t *testing.T) {
	vb := NewVirtualBackend(3, 1)
	f := NewFrame(3, 1)
	f.Set(0, 0, Cell{Content: "a"})
	if err := vb.Present(f); err != nil {
		t.Fatal(err)
	}
	f.Set(0, 0, Cell{Content: "b"})
	if err := vb.Present(f); err != nil {
		t.Fatal(err)
	}
	if vb.Presents() != 2 || vb.FrameCount() != 2 {
		t.Fatalf("Presents/FrameCount = %d/%d", vb.Presents(), vb.FrameCount())
	}
	// history holds clones: frame 0 still shows "a" after the mutation
	if got := strings.Join(vb.ScreenAt(0), ""); got != "a  " {
		t.Fatalf("ScreenAt(0) = %q", got)
	}
	if got := strings.Join(vb.Screen(), ""); got != "b  " {
		t.Fatalf("Screen() = %q", got)
	}
	if c, ok := vb.CellAt(0, 0); !ok || c.Content != "b" {
		t.Fatalf("CellAt = %+v, %v", c, ok)
	}
	if _, ok := vb.CellAt(9, 9); ok {
		t.Fatal("CellAt out of bounds reported ok")
	}
	if vb.ScreenAt(-1) != nil || vb.ScreenAt(2) != nil {
		t.Fatal("ScreenAt out of range returned rows")
	}
	vb.PresentErr = errors.New("nope")
	if err := vb.Present(f); err == nil {
		t.Fatal("PresentErr knob ignored")
	}
}

func TestVirtualPresentHistoryIsBounded(t *testing.T) {
	vb := NewVirtualBackend(1, 1)
	f := NewFrame(1, 1)
	for i := 0; i < defaultMaxFrames+10; i++ {
		if err := vb.Present(f); err != nil {
			t.Fatal(err)
		}
	}
	if vb.FrameCount() != defaultMaxFrames {
		t.Fatalf("history grew past bound: %d", vb.FrameCount())
	}
	if vb.Presents() != defaultMaxFrames+10 {
		t.Fatalf("Presents = %d", vb.Presents())
	}
}
