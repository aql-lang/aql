package formatter

import (
	"strings"
	"testing"
)

func TestFormatBasic(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple def",
			in:   "def x 42\n",
			want: "def x 42\n",
		},
		{
			name: "import",
			in:   `(import "textkit")` + "\n",
			want: `(import "textkit")` + "\n",
		},
		{
			name: "fn sig unwrap",
			in:   "def square fn [[x:Integer] [Integer] [x mul x]]\n",
			want: "def square fn [x:Integer] [Integer] [x mul x]\n",
		},
		{
			name: "type capitalize after colon",
			in:   "def f fn [[x:string] [integer] [x]]\n",
			want: "def f fn [x:String] [Integer] [x]\n",
		},
		{
			name: "no capitalize variable names",
			in:   "def table 42\ndef node 7\n",
			want: "def table 42\ndef node 7\n",
		},
		{
			name: "map formatting",
			in:   "{a:1 b:2 c:3}\n",
			want: "{a: 1 b: 2 c: 3}\n",
		},
		{
			name: "export map",
			in:   "export Foo {a:1 b:2}\n",
			want: "export Foo {a: 1 b: 2}\n",
		},
		{
			name: "empty list",
			in:   "[]\n",
			want: "[]\n",
		},
		{
			name: "empty map",
			in:   "{}\n",
			want: "{}\n",
		},
		{
			name: "record keyword not capitalized",
			in:   "type Cond record [field:Atom op:String]\n",
			want: "type Cond record [field:Atom op:String]\n",
		},
		{
			name: "comment preserved",
			in:   "# hello world\ndef x 1\n",
			want: "# hello world\ndef x 1\n",
		},
		{
			name: "no tabs in output",
			in:   "\tdef x\t42\n",
			want: "def x 42\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.in)
			if got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatNoTabs(t *testing.T) {
	src := "def f fn [[x:String] [String] [x upper]]\nexport Foo {a:1 b:2 c:3}\n"
	result := Format(src)
	if strings.Contains(result, "\t") {
		t.Errorf("formatted output contains tabs:\n%s", result)
	}
}

func TestFormatLineWidth(t *testing.T) {
	src := `def long-fn fn [[a:String b:String c:String d:String e:String] [String] [a add b add c add d add e]]` + "\n"
	result := Format(src)
	for i, line := range strings.Split(result, "\n") {
		if len(line) > maxLineWidth {
			t.Errorf("line %d exceeds %d chars (%d): %q",
				i+1, maxLineWidth, len(line), line)
		}
	}
}

func TestFormatIdempotent(t *testing.T) {
	src := `(import "textkit")

def process fn [[s:String] [String] [(s Styler.style)]]

export Textkit {process: process}
`
	first := Format(src)
	second := Format(first)
	if first != second {
		t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}
