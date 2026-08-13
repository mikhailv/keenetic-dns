package blocklist

import (
	"bufio"
	"fmt"
	"io"
	"iter"
	"strings"
)

type Action uint8

const (
	Block Action = iota
	Allow
)

func (a Action) String() string {
	if a == Allow {
		return "allow"
	}
	return "block"
}

func (a Action) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a *Action) UnmarshalText(text []byte) error {
	switch string(text) {
	case "", "block":
		*a = Block
	case "allow":
		*a = Allow
	default:
		return fmt.Errorf("unknown action %q, want \"block\" or \"allow\"", text)
	}
	return nil
}

type Rule struct {
	Domain     string
	Action     Action
	Subdomains bool
}

const maxLineSize = 64 * 1024

func Parse(r io.Reader) iter.Seq2[Rule, error] {
	return func(yield func(Rule, error) bool) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 4096), maxLineSize)
		var buf []Rule
		for sc.Scan() {
			buf = parseLine(sc.Text(), buf[:0])
			for _, rule := range buf {
				if !yield(rule, nil) {
					return
				}
			}
		}
		if err := sc.Err(); err != nil {
			yield(Rule{}, err)
		}
	}
}

var sinkIPs = map[string]bool{
	"0.0.0.0":         true,
	"127.0.0.1":       true,
	"255.255.255.255": true,
	"::":              true,
	"::1":             true,
}

var localNames = map[string]bool{
	"localhost":             true,
	"localhost.localdomain": true,
	"local":                 true,
	"broadcasthost":         true,
	"ip6-localhost":         true,
	"ip6-loopback":          true,
	"ip6-localnet":          true,
	"ip6-mcastprefix":       true,
	"ip6-allnodes":          true,
	"ip6-allrouters":        true,
	"ip6-allhosts":          true,
}

var safeAdblockModifiers = map[string]bool{
	"all":       true,
	"important": true,
	"document":  true,
	"doc":       true,
	"popup":     true,
}

func parseLine(line string, dst []Rule) []Rule {
	line = strings.TrimSpace(line)
	if line == "" {
		return dst
	}
	switch line[0] {
	case '#', '!', '[': // comment, AdBlock comment, AdBlock header
		return dst
	}
	if strings.Contains(line, "##") || strings.Contains(line, "#@#") || strings.Contains(line, "#?#") {
		return dst
	}
	switch {
	case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "||"):
		return parseAdblockLine(line, dst)
	case strings.HasPrefix(line, "address=/"), strings.HasPrefix(line, "local=/"):
		return parseDnsmasqLine(line, dst)
	}
	line = trimInlineComment(line)
	if line == "" {
		return dst
	}
	if fields := strings.Fields(line); len(fields) > 1 {
		return parseHostsLine(fields, dst)
	}
	return parsePlainLine(line, dst)
}

func parseHostsLine(fields []string, dst []Rule) []Rule {
	if !sinkIPs[fields[0]] {
		return dst
	}
	for _, field := range fields[1:] {
		if domain, ok := normalizeDomain(field); ok {
			dst = append(dst, Rule{Domain: domain, Action: Block})
		}
	}
	return dst
}

func parsePlainLine(line string, dst []Rule) []Rule {
	line = strings.TrimSuffix(line, "^")
	line = strings.TrimPrefix(line, "*.")
	line = strings.TrimPrefix(line, ".")
	if domain, ok := normalizeDomain(line); ok {
		dst = append(dst, Rule{Domain: domain, Action: Block, Subdomains: true})
	}
	return dst
}

func parseAdblockLine(line string, dst []Rule) []Rule {
	action := Block
	if rest, ok := strings.CutPrefix(line, "@@"); ok {
		action = Allow
		line = rest
	}
	line, ok := strings.CutPrefix(line, "||")
	if !ok {
		return dst
	}
	line, ok = cutAdblockModifiers(line)
	if !ok {
		return dst
	}
	line = strings.TrimSuffix(line, "|")
	line = strings.TrimSuffix(line, "^")
	if strings.ContainsAny(line, "/^") {
		return dst
	}
	if domain, ok := normalizeDomain(line); ok {
		dst = append(dst, Rule{Domain: domain, Action: action, Subdomains: true})
	}
	return dst
}

func cutAdblockModifiers(rule string) (string, bool) {
	pattern, modifiers, found := strings.Cut(rule, "$")
	if !found {
		return rule, true
	}
	for modifier := range strings.SplitSeq(modifiers, ",") {
		name, _, _ := strings.Cut(modifier, "=")
		if !safeAdblockModifiers[strings.TrimSpace(name)] {
			return "", false
		}
	}
	return pattern, true
}

func parseDnsmasqLine(line string, dst []Rule) []Rule {
	_, rest, _ := strings.Cut(line, "=/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return dst
	}
	target := strings.TrimSpace(parts[len(parts)-1])
	if target != "" && target != "#" && !sinkIPs[target] {
		return dst
	}
	for _, part := range parts[:len(parts)-1] {
		if domain, ok := normalizeDomain(part); ok {
			dst = append(dst, Rule{Domain: domain, Action: Block, Subdomains: true})
		}
	}
	return dst
}

func trimInlineComment(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func normalizeDomain(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	if s == "" || len(s) > 253 || localNames[s] {
		return "", false
	}
	var last string
	var alnum bool
	for label := range strings.SplitSeq(s, ".") {
		if label == "" || len(label) > 63 || !validLabel(label) {
			return "", false
		}
		alnum = alnum || hasAlnum(label)
		last = label
	}
	if !alnum {
		return "", false
	}
	if isNumeric(last) {
		return "", false
	}
	return s, true
}

func validLabel(label string) bool {
	for i := range len(label) {
		switch c := label[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '*':
		default:
			return false
		}
	}
	return true
}

func hasAlnum(label string) bool {
	for i := range len(label) {
		if c := label[i]; c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func isNumeric(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
