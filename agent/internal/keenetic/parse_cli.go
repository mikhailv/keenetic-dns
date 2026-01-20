package keenetic

import (
	"fmt"
	"iter"
	"strings"
)

func ParseOutput(output string) []Object {
	var result []Object

	type stackItem struct {
		indent int
		obj    Object
	}

	var stack []stackItem

	for cur, next := range iterateParsedLinePairs(iterateParsedLines(iterateLines(output))) {
		for len(stack) > 0 && stack[len(stack)-1].indent > cur.indent {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			if cur.value != "" {
				panic(fmt.Errorf("[ndm] unexpected object line: %q", cur.raw))
			}
			if next.indent <= cur.indent {
				panic(fmt.Errorf("[ndm] unexpected indent of lines: %q and %q", cur.raw, next.raw))
			}
			obj := Object{}
			stack = append(stack, stackItem{indent: next.indent, obj: obj})
			result = append(result, Object{cur.key: obj})
			continue
		}

		parent := stack[len(stack)-1]
		if parent.indent == cur.indent {
			if next.indent > cur.indent { // new object
				if cur.value != "" {
					panic(fmt.Errorf("[ndm] unexpected start of new object: %q and %q", cur.raw, next.raw))
				}
				obj := Object{}
				stack = append(stack, stackItem{indent: next.indent, obj: obj})
				parent.obj[cur.key] = obj
			} else {
				parent.obj[cur.key] = cur.value
			}
			continue
		}
	}

	return result
}

func iterateLines(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			np := strings.IndexByte(s, '\n')
			var line string
			if np == -1 {
				line = s
			} else {
				line = strings.TrimRight(s[:np], "\r")
			}
			if !yield(line) || np == -1 {
				return
			}
			s = s[np+1:]
		}
	}
}

type parsedLine struct {
	raw        string
	key, value string
	indent     int
}

func iterateParsedLines(lines iter.Seq[string]) iter.Seq[parsedLine] {
	return func(yield func(parsedLine) bool) {
		for line := range lines {
			if strings.TrimSpace(line) == "" || line == "\u001B[K" {
				continue
			}

			parsed := parsedLine{
				raw: line,
			}

			p := strings.Index(line, ": ")
			if p > 0 {
				parsed.key = strings.TrimSpace(line[:p])
				parsed.value = strings.TrimSpace(line[p+2:])
				parsed.indent = p + 2
			}
			if !yield(parsed) {
				return
			}
		}
	}
}

func iterateParsedLinePairs(lines iter.Seq[parsedLine]) iter.Seq2[parsedLine, parsedLine] {
	return func(yield func(parsedLine, parsedLine) bool) {
		next, stop := iter.Pull(lines)
		defer stop()

		line1, ok := next()
		if !ok {
			return
		}

		for {
			line2, ok := next()
			if !ok {
				yield(line1, line1) // yield last line
				return
			}
			if line2.key == "" && line2.value == "" {
				if len(line2.raw) <= line1.indent || strings.TrimSpace(line2.raw[:line1.indent]) != "" {
					panic(fmt.Errorf("[ndm] unexpected line: %q", line2.raw))
				}
				line1.value += strings.TrimRight(line2.raw[line1.indent:], " ")
				continue
			}

			if !yield(line1, line2) {
				return
			}
			line1 = line2
		}
	}
}
