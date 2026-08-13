package blocklist

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blevesearch/vellum"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

const buildWriteBuffer = 256 * 1024

type Builder struct {
	entries []entry
	lists   []listInfo
	regexps []ManifestRegexp
	state   []ListState
}

func (b *Builder) SetState(state []ListState) { b.state = state }

type listInfo struct {
	name   string
	source string
}

type entry struct {
	key        string // reversed labels
	blockMask  uint32
	allowMask  uint32
	subdomains bool
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) AddList(name, source string) (int, error) {
	if len(b.lists) >= MaxLists {
		return 0, fmt.Errorf("too many lists, at most %d are supported", MaxLists)
	}
	b.lists = append(b.lists, listInfo{name: name, source: source})
	return len(b.lists) - 1, nil
}

func (b *Builder) AddDomain(domain string, action Action, listID int) bool {
	normalized, ok := normalizeDomain(strings.TrimPrefix(domain, "*."))
	if !ok {
		return false
	}
	b.Add(Rule{Domain: normalized, Action: action, Subdomains: true}, listID)
	return true
}

func (b *Builder) Add(rule Rule, listID int) {
	if listID < 0 || listID >= len(b.lists) {
		return
	}
	if strings.Contains(rule.Domain, "*") {
		b.addRegex(rule, listID)
		return
	}
	e := entry{key: reverseLabels(rule.Domain), subdomains: rule.Subdomains}
	if rule.Action == Allow {
		e.allowMask = uint32(1) << listID
	} else {
		e.blockMask = uint32(1) << listID
	}
	b.entries = append(b.entries, e)
}

func (b *Builder) addRegex(rule Rule, listID int) {
	var sb strings.Builder
	sb.WriteString(`(?i)^`)
	if rule.Subdomains {
		sb.WriteString(`(?:[^.]+\.)*`)
	}
	for i, part := range strings.Split(rule.Domain, "*") {
		if i > 0 {
			sb.WriteString(`[^.]*`)
		}
		sb.WriteString(regexp.QuoteMeta(part))
	}
	sb.WriteString(`\.?$`)
	b.regexps = append(b.regexps, ManifestRegexp{
		Pattern: sb.String(),
		ListID:  listID,
		Allow:   rule.Action == Allow,
	})
}

func (b *Builder) RuleCount() int { return len(b.entries) }

func (b *Builder) RegexpCount() int { return len(b.regexps) }

func (b *Builder) Write(indexPath string, builtAt time.Time) error {
	b.dedupe()

	names := make([]string, len(b.lists))
	sources := map[string]string{}
	for i, list := range b.lists {
		names[i] = list.name
		if list.source != "" {
			sources[list.name] = list.source
		}
	}
	manifest := &Manifest{
		Lists:   names,
		Regexps: b.regexps,
		Domains: len(b.entries),
		Sources: sources,
		State:   b.state,
		BuiltAt: builtAt.UTC().Format(time.RFC3339),
	}

	return b.writeFST(indexPath, func() error {
		return manifest.save(ManifestPath(indexPath))
	})
}

func (b *Builder) dedupe() {
	slices.SortFunc(b.entries, func(x, y entry) int {
		return strings.Compare(x.key, y.key)
	})
	merged := b.entries[:0]
	for _, e := range b.entries {
		if n := len(merged); n > 0 && merged[n-1].key == e.key {
			merged[n-1].blockMask |= e.blockMask
			merged[n-1].allowMask |= e.allowMask
			merged[n-1].subdomains = merged[n-1].subdomains || e.subdomains
			continue
		}
		merged = append(merged, e)
	}
	b.entries = merged
}

func (b *Builder) writeFST(path string, beforeCommit func() error) error {
	return util.SaveToFile(path, util.SaveFileConfig{
		Saver: func(fw io.Writer) error {
			w := bufio.NewWriterSize(fw, buildWriteBuffer)
			builder, err := vellum.New(w, nil)
			if err != nil {
				return fmt.Errorf("create index builder: %w", err)
			}
			for _, e := range b.entries {
				value := packValue(e.blockMask, e.allowMask, e.subdomains)
				if err := builder.Insert([]byte(e.key), value); err != nil {
					return fmt.Errorf("insert %q: %w", e.key, err)
				}
			}
			if err := builder.Close(); err != nil {
				return fmt.Errorf("finish index: %w", err)
			}
			return w.Flush()
		},
		BeforeCommit: func(_ string) error {
			return beforeCommit()
		},
	})
}
