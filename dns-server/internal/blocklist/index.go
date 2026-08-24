package blocklist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/blevesearch/vellum"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	MaxLists = 15

	blockSubShift   = 0
	blockExactShift = MaxLists
	allowSubShift   = MaxLists * 2
	allowExactShift = MaxLists * 3

	maskBits   = uint64(1<<MaxLists) - 1
	maskBits32 = uint32(1<<MaxLists) - 1
)

type listMasks struct {
	blockSub, blockExact, allowSub, allowExact uint32
}

func (m listMasks) or(o listMasks) listMasks {
	return listMasks{
		blockSub:   m.blockSub | o.blockSub,
		blockExact: m.blockExact | o.blockExact,
		allowSub:   m.allowSub | o.allowSub,
		allowExact: m.allowExact | o.allowExact,
	}
}

func packValue(m listMasks) uint64 {
	return uint64(m.blockSub)&maskBits<<blockSubShift |
		uint64(m.blockExact)&maskBits<<blockExactShift |
		uint64(m.allowSub)&maskBits<<allowSubShift |
		uint64(m.allowExact)&maskBits<<allowExactShift
}

func unpackValue(v uint64) listMasks {
	return listMasks{
		blockSub:   uint32(v >> blockSubShift & maskBits),
		blockExact: uint32(v >> blockExactShift & maskBits),
		allowSub:   uint32(v >> allowSubShift & maskBits),
		allowExact: uint32(v >> allowExactShift & maskBits),
	}
}

type Match struct {
	List    string
	Action  Action
	Pattern string
}

func (m Match) Blocked() bool { return m.Action == Block }

const manifestVersion = 1

type Manifest struct {
	Version int               `json:"version"`
	Lists   []string          `json:"lists"`
	Regexps []ManifestRegexp  `json:"regexps,omitempty"`
	Domains int               `json:"domains"`
	Sources map[string]string `json:"sources,omitempty"`
	State   []ListState       `json:"state,omitempty"`
	BuiltAt string            `json:"built_at"`
}

type ManifestRegexp struct {
	Pattern string `json:"pattern"`
	ListID  int    `json:"list_id"`
	Allow   bool   `json:"allow,omitempty"`
}

type ListState struct {
	Name      string   `json:"name"`
	URLs      []string `json:"urls,omitempty"`
	AllowURLs []string `json:"allow_urls,omitempty"`
	Allow     []string `json:"allow,omitempty"`
	Deny      []string `json:"deny,omitempty"`
}

func ManifestPath(indexPath string) string {
	return strings.TrimSuffix(indexPath, ".fst") + ".json"
}

func (m *Manifest) save(path string) error {
	return util.SaveToFileFunc(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(m); err != nil {
			return fmt.Errorf("encode manifest: %w", err)
		}
		return nil
	})
}

func loadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &m, nil
}

type regexRule struct {
	re     *regexp.Regexp
	listID int
	action Action
}

type Index struct {
	fst      *vellum.FST
	manifest *Manifest
	regexps  []regexRule
	lookups  sync.Pool
}

type lookupState struct {
	reader *vellum.Reader
	key    []byte
}

func Open(path string) (*Index, error) {
	manifest, err := loadManifest(ManifestPath(path))
	if err != nil {
		return nil, err
	}
	if manifest.Version != manifestVersion {
		return nil, fmt.Errorf("index format %d, want %d", manifest.Version, manifestVersion)
	}
	fst, err := vellum.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	idx := &Index{fst: fst, manifest: manifest}
	idx.lookups.New = func() any {
		reader, _ := fst.Reader()
		return &lookupState{reader: reader, key: make([]byte, 0, 256)}
	}
	for _, r := range manifest.Regexps {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			continue
		}
		action := Block
		if r.Allow {
			action = Allow
		}
		idx.regexps = append(idx.regexps, regexRule{re: re, listID: r.ListID, action: action})
	}
	return idx, nil
}

func (idx *Index) Close() error {
	if idx == nil || idx.fst == nil {
		return nil
	}
	return idx.fst.Close()
}

func (idx *Index) Domains() int { return idx.manifest.Domains }

func (idx *Index) BuiltAt() string { return idx.manifest.BuiltAt }

func (idx *Index) State() []ListState { return idx.manifest.State }

func (idx *Index) ListIDs() map[string]int {
	ids := make(map[string]int, len(idx.manifest.Lists))
	for i, name := range idx.manifest.Lists {
		ids[name] = i
	}
	return ids
}

func (idx *Index) Lookup(domain string, clientMask uint32) (Match, bool) {
	st, _ := idx.lookups.Get().(*lookupState)
	defer idx.lookups.Put(st)

	domain = normalizeLookup(domain)
	st.key = appendReversed(st.key[:0], domain)
	if len(st.key) == 0 {
		return Match{}, false
	}
	if value, exact, ok := idx.lookupFST(st, clientMask); ok {
		return idx.match(value, exact, clientMask), true
	}
	for _, r := range idx.regexps {
		if clientMask&(uint32(1)<<r.listID) == 0 {
			continue
		}
		if r.re.MatchString(domain) {
			return Match{List: idx.listName(r.listID), Action: r.action, Pattern: r.re.String()}, true
		}
	}
	return Match{}, false
}

func (idx *Index) match(value uint64, exact bool, clientMask uint32) Match {
	block, allow := applicableMasks(unpackValue(value), exact)
	block &= clientMask
	allow &= clientMask

	if allow != 0 {
		return Match{List: idx.listName(lowestBit(allow)), Action: Allow}
	}
	return Match{List: idx.listName(lowestBit(block)), Action: Block}
}

func applicableMasks(m listMasks, exact bool) (block, allow uint32) {
	block, allow = m.blockSub, m.allowSub
	if exact {
		block |= m.blockExact
		allow |= m.allowExact
	}
	return block, allow
}

func lowestBit(mask uint32) int {
	return bits.TrailingZeros32(mask)
}

func (idx *Index) lookupFST(st *lookupState, clientMask uint32) (value uint64, exact, found bool) {
	for end := len(st.key); end > 0; {
		v, ok, err := st.reader.Get(st.key[:end])
		if err == nil && ok {
			atEnd := end == len(st.key)
			block, allow := applicableMasks(unpackValue(v), atEnd)
			if (block|allow)&clientMask != 0 {
				return v, atEnd, true
			}
		}
		i := bytes.LastIndexByte(st.key[:end], '.')
		if i < 0 {
			break
		}
		end = i
	}
	return 0, false, false
}

func (idx *Index) listName(listID int) string {
	if listID < 0 || listID >= len(idx.manifest.Lists) {
		return ""
	}
	return idx.manifest.Lists[listID]
}

func normalizeLookup(domain string) string {
	return util.TrimFQDN(strings.TrimSpace(domain))
}

func appendReversed(dst []byte, domain string) []byte {
	end := len(domain)
	for i := len(domain) - 1; i >= 0; i-- {
		if domain[i] != '.' {
			continue
		}
		dst = appendLower(dst, domain[i+1:end])
		dst = append(dst, '.')
		end = i
	}
	return appendLower(dst, domain[:end])
}

func appendLower(dst []byte, s string) []byte {
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

func reverseLabels(domain string) string {
	if domain == "" {
		return ""
	}
	n := strings.Count(domain, ".")
	if n == 0 {
		return domain
	}
	var sb strings.Builder
	sb.Grow(len(domain))
	end := len(domain)
	for i := len(domain) - 1; i >= 0; i-- {
		if domain[i] != '.' {
			continue
		}
		sb.WriteString(domain[i+1 : end])
		sb.WriteByte('.')
		end = i
	}
	sb.WriteString(domain[:end])
	return sb.String()
}
