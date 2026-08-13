package blocklist

import (
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"os"
	"regexp"
	"strings"

	"github.com/blevesearch/vellum"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	flagSubdomains uint64 = 1 << 0
	blockMaskShift        = 1
	allowMaskShift        = 32
	MaxLists              = 31

	maskBits   = uint64(1<<MaxLists) - 1
	maskBits32 = uint32(1<<MaxLists) - 1
)

func packValue(blockMask, allowMask uint32, subdomains bool) uint64 {
	var v uint64
	if subdomains {
		v |= flagSubdomains
	}
	v |= (uint64(blockMask) & maskBits) << blockMaskShift
	v |= (uint64(allowMask) & maskBits) << allowMaskShift
	return v
}

func unpackValue(v uint64) (blockMask, allowMask uint32, subdomains bool) {
	blockMask = uint32((v >> blockMaskShift) & maskBits)
	allowMask = uint32((v >> allowMaskShift) & maskBits)
	return blockMask, allowMask, v&flagSubdomains != 0
}

type Match struct {
	List    string
	Action  Action
	Pattern string
}

func (m Match) Blocked() bool { return m.Action == Block }

type Manifest struct {
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
}

func Open(path string) (*Index, error) {
	manifest, err := loadManifest(ManifestPath(path))
	if err != nil {
		return nil, err
	}
	fst, err := vellum.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	idx := &Index{fst: fst, manifest: manifest}
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
	rev := reverseLabels(normalizeLookup(domain))
	if rev == "" {
		return Match{}, false
	}
	if value, ok := idx.lookupFST(rev, clientMask); ok {
		return idx.match(value, clientMask), true
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

func (idx *Index) match(value uint64, clientMask uint32) Match {
	blockMask, allowMask, _ := unpackValue(value)
	blockMask &= clientMask
	allowMask &= clientMask

	if allowMask != 0 {
		return Match{List: idx.listName(lowestBit(allowMask)), Action: Allow}
	}
	return Match{List: idx.listName(lowestBit(blockMask)), Action: Block}
}

func lowestBit(mask uint32) int {
	return bits.TrailingZeros32(mask)
}

func (idx *Index) lookupFST(rev string, clientMask uint32) (uint64, bool) {
	addr := idx.fst.Start()
	var acc, best uint64
	var found bool

	for i := 0; ; i++ {
		atEnd := i == len(rev)
		if atEnd || rev[i] == '.' {
			if matched, out := idx.fst.IsMatchWithVal(addr); matched {
				value := acc + out
				blockMask, allowMask, subdomains := unpackValue(value)
				if (atEnd || subdomains) && (blockMask|allowMask)&clientMask != 0 {
					best, found = value, true
				}
			}
		}
		if atEnd {
			return best, found
		}
		var out uint64
		addr, out = idx.fst.AcceptWithVal(addr, rev[i])
		if !idx.fst.CanMatch(addr) {
			return best, found
		}
		acc += out
	}
}

func (idx *Index) listName(listID int) string {
	if listID < 0 || listID >= len(idx.manifest.Lists) {
		return ""
	}
	return idx.manifest.Lists[listID]
}

func normalizeLookup(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
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
