// Temporary differential probe for SafeParseData. Reads escaped inputs
// (one per line: \n \t \\ decoded) from stdin, prints one render per line.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	jsonic "github.com/tabnas/jsonic/go"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

func decode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func render(v any) (string, error) {
	if ev, claimed, cerr := parser.ConvertParsedNumber(v); claimed {
		if cerr != nil {
			return "", cerr
		}
		return core.CanonValue(ev), nil
	}
	switch x := v.(type) {
	case nil:
		return "none", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case string:
		return "'" + x + "'", nil
	case []any:
		items := make([]string, 0, len(x))
		for _, item := range x {
			s, err := render(item)
			if err != nil {
				return "", err
			}
			items = append(items, s)
		}
		return "[" + strings.Join(items, " ") + "]", nil
	case *jsonic.OrderedMap:
		pairs := make([]string, 0, len(x.Keys))
		for _, k := range x.Keys {
			child, _ := x.Get(k)
			s, err := render(child)
			if err != nil {
				return "", err
			}
			pairs = append(pairs, k+":"+s)
		}
		return "{" + strings.Join(pairs, " ") + "}", nil
	}
	return "", fmt.Errorf("UNSUPPORTED %T", v)
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for sc.Scan() {
		src := decode(sc.Text())
		v, err := parser.SafeParseData(src)
		if err != nil {
			fmt.Fprintln(out, "ERR")
			continue
		}
		s, cerr := render(v)
		if cerr != nil {
			var be *core.BoruError
			if errors.As(cerr, &be) {
				fmt.Fprintln(out, "ERR "+be.Code)
			} else {
				fmt.Fprintln(out, "HARNESS "+cerr.Error())
			}
			continue
		}
		fmt.Fprintln(out, s)
	}
}
