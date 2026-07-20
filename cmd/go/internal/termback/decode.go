package termback

import (
	"bufio"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/aql-lang/aql/lang/go/tuikit"
)

// decodeInput turns the raw-mode byte stream into tuikit events. It
// runs until read error/EOF; the CALLER closes the channel after it
// returns (the backend does so under its event latch, so the resize
// watcher's concurrent sends can never hit a closed channel). A Close
// on the backend flips `closed` so late events are dropped rather than
// delivered to a restored terminal. A lone ESC is delivered as the esc
// key when no continuation byte is already buffered (modern terminals
// send CSI sequences atomically; there is no ESC timeout).
func decodeInput(br *bufio.Reader, events chan<- tuikit.Event, closed *atomic.Bool) {
	deliver := func(ev tuikit.Event) bool {
		if closed.Load() {
			return false
		}
		events <- ev
		return true
	}
	for {
		r, _, err := br.ReadRune()
		if err != nil {
			return
		}
		var ev tuikit.Event
		switch {
		case r == 0x1b:
			seq, ok := readEscape(br)
			if !ok {
				return
			}
			ev = seq
		case r == '\r' || r == '\n':
			ev = tuikit.Event{Tag: "key", Key: "enter"}
		case r == '\t':
			ev = tuikit.Event{Tag: "key", Key: "tab"}
		case r == 0x7f || r == 0x08:
			ev = tuikit.Event{Tag: "key", Key: "backspace"}
		case r == ' ':
			ev = tuikit.Event{Tag: "key", Key: "space", Char: " "}
		case r < 0x20: // ctrl chords: 0x01-0x1a → ctrl+a..z, 0x1c → ctrl+\
			ev = ctrlEvent(r)
		default:
			ev = tuikit.Event{Tag: "key", Key: string(r), Char: string(r)}
		}
		if ev.Tag == "" {
			continue // an unrecognised sequence is dropped, not delivered
		}
		if !deliver(ev) {
			return
		}
	}
}

func ctrlEvent(r rune) tuikit.Event {
	switch {
	case r >= 0x01 && r <= 0x1a:
		return tuikit.Event{Tag: "key", Key: string('a' + r - 1), Mods: []string{"ctrl"}}
	case r == 0x1c:
		return tuikit.Event{Tag: "key", Key: `\`, Mods: []string{"ctrl"}}
	default:
		return tuikit.Event{}
	}
}

// csiKeys maps CSI final letters and tilde codes to key names.
var csiLetterKeys = map[byte]string{
	'A': "up", 'B': "down", 'C': "right", 'D': "left",
	'H': "home", 'F': "end", 'Z': "backtab",
}

var csiTildeKeys = map[string]string{
	"1": "home", "2": "insert", "3": "delete", "4": "end",
	"5": "pgup", "6": "pgdn",
	"11": "f1", "12": "f2", "13": "f3", "14": "f4",
	"15": "f5", "17": "f6", "18": "f7", "19": "f8",
	"20": "f9", "21": "f10", "23": "f11", "24": "f12",
}

// readEscape consumes one escape sequence (the ESC is already read).
// The bool is false only on read error/EOF mid-sequence with nothing to
// deliver.
func readEscape(br *bufio.Reader) (tuikit.Event, bool) {
	if br.Buffered() == 0 {
		return tuikit.Event{Tag: "key", Key: "esc"}, true // a lone ESC keypress
	}
	b, _ := br.ReadByte() // Buffered()>0 ⇒ the read cannot fail
	switch b {
	case '[':
		return readCSI(br)
	case 'O': // SS3 (application cursor keys / F1-F4)
		fb, err := br.ReadByte()
		if err != nil {
			return tuikit.Event{}, false
		}
		switch fb {
		case 'A', 'B', 'C', 'D', 'H', 'F':
			return tuikit.Event{Tag: "key", Key: csiLetterKeys[fb]}, true
		case 'P', 'Q', 'R', 'S':
			return tuikit.Event{Tag: "key", Key: "f" + string(rune('1'+fb-'P'))}, true
		}
		return tuikit.Event{}, true
	default: // ESC-prefixed rune = alt chord
		return tuikit.Event{Tag: "key", Key: string(rune(b)), Char: string(rune(b)), Mods: []string{"alt"}}, true
	}
}

// readCSI parses one CSI sequence: parameters, then a final byte.
func readCSI(br *bufio.Reader) (tuikit.Event, bool) {
	var params strings.Builder
	for {
		b, err := br.ReadByte()
		if err != nil {
			return tuikit.Event{}, false
		}
		if b >= 0x40 && b <= 0x7e { // the final byte
			return csiEvent(params.String(), b, br)
		}
		params.WriteByte(b)
	}
}

func csiEvent(params string, final byte, br *bufio.Reader) (tuikit.Event, bool) {
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F', 'Z':
		return tuikit.Event{Tag: "key", Key: csiLetterKeys[final]}, true
	case '~':
		// bracketed paste: ESC[200~ … ESC[201~
		if params == "200" {
			return readPaste(br)
		}
		if key, ok := csiTildeKeys[strings.Split(params, ";")[0]]; ok {
			return tuikit.Event{Tag: "key", Key: key}, true
		}
		return tuikit.Event{}, true
	case 'M', 'm':
		return sgrMouse(params, final == 'M'), true
	default:
		return tuikit.Event{}, true // unhandled CSI: dropped
	}
}

// readPaste consumes bracketed-paste content through the 201~ closer.
func readPaste(br *bufio.Reader) (tuikit.Event, bool) {
	var text strings.Builder
	for {
		r, _, err := br.ReadRune()
		if err != nil {
			return tuikit.Event{}, false
		}
		if r == 0x1b {
			// expect [201~
			rest := make([]byte, 5)
			if _, err := br.Read(rest); err != nil {
				return tuikit.Event{}, false
			}
			return tuikit.Event{Tag: "paste", Text: text.String()}, true
		}
		text.WriteRune(r)
	}
}

// sgrMouse decodes an SGR-1006 mouse report: "<btn;x;y".
func sgrMouse(params string, press bool) tuikit.Event {
	trimmed := strings.TrimPrefix(params, "<")
	parts := strings.Split(trimmed, ";")
	if len(parts) != 3 {
		return tuikit.Event{}
	}
	btn, err1 := strconv.Atoi(parts[0])
	x, err2 := strconv.Atoi(parts[1])
	y, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return tuikit.Event{}
	}
	kind := "press"
	switch {
	case btn&64 != 0:
		if btn&1 != 0 {
			kind = "wheel-down"
		} else {
			kind = "wheel-up"
		}
	case btn&32 != 0:
		kind = "move"
	case !press:
		kind = "release"
	}
	return tuikit.Event{Tag: "mouse", Kind: kind, X: x - 1, Y: y - 1}
}
