package keenetic

import (
	"fmt"
	"iter"
	"strings"
)

func ParseOutput(output string) ([]Object, error) {
	var result []Object

	type stackItem struct {
		indent int
		obj    Object
	}

	var stack []stackItem

	for pair, err := range iterateParsedLinePairs(iterateParsedLines(iterateLines(output))) {
		if err != nil {
			return nil, err
		}
		cur, next := pair[0], pair[1]
		for len(stack) > 0 && stack[len(stack)-1].indent > cur.indent {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			if cur.value != "" {
				return nil, fmt.Errorf("[ndm] unexpected object line: %q", cur.raw)
			}
			if next.indent <= cur.indent {
				return nil, fmt.Errorf("[ndm] unexpected indent of lines: %q and %q", cur.raw, next.raw)
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
					return nil, fmt.Errorf("[ndm] unexpected start of new object: %q and %q", cur.raw, next.raw)
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

	return result, nil
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

func iterateParsedLinePairs(lines iter.Seq[parsedLine]) iter.Seq2[[2]parsedLine, error] {
	return func(yield func([2]parsedLine, error) bool) {
		next, stop := iter.Pull(lines)
		defer stop()

		line1, ok := next()
		if !ok {
			return
		}

		for {
			line2, ok := next()
			if !ok {
				yield([2]parsedLine{line1, line1}, nil) // yield last line
				return
			}
			if line2.key == "" && line2.value == "" {
				if len(line2.raw) <= line1.indent || strings.TrimSpace(line2.raw[:line1.indent]) != "" {
					yield([2]parsedLine{line1, line2}, fmt.Errorf("[ndm] unexpected next line: %q", line2.raw))
					return
				}
				line1.value += strings.TrimRight(line2.raw[line1.indent:], " ")
				continue
			}

			if !yield([2]parsedLine{line1, line2}, nil) {
				return
			}
			line1 = line2
		}
	}
}
