package blocklist

import (
	"bufio"
	"iter"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseAll(t *testing.T, input string) []Rule {
	t.Helper()
	var rules []Rule
	for rule, err := range Parse(strings.NewReader(input)) {
		require.NoError(t, err)
		rules = append(rules, rule)
	}
	return rules
}

func TestParse_Hosts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Rule
	}{
		{
			name:  "sink 0.0.0.0",
			input: "0.0.0.0 ads.example.com",
			want:  []Rule{{Domain: "ads.example.com", Action: Block}},
		},
		{
			name:  "sink 127.0.0.1",
			input: "127.0.0.1 ads.example.com",
			want:  []Rule{{Domain: "ads.example.com", Action: Block}},
		},
		{
			name:  "several domains on one line",
			input: "0.0.0.0 a.example.com b.example.com",
			want: []Rule{
				{Domain: "a.example.com", Action: Block},
				{Domain: "b.example.com", Action: Block},
			},
		},
		{
			name:  "tab separated",
			input: "0.0.0.0\tads.example.com",
			want:  []Rule{{Domain: "ads.example.com", Action: Block}},
		},
		{
			name:  "inline comment stripped",
			input: "0.0.0.0 ads.example.com # tracker",
			want:  []Rule{{Domain: "ads.example.com", Action: Block}},
		},
		{
			name:  "localhost entries skipped",
			input: "127.0.0.1 localhost\n::1 ip6-localhost\n255.255.255.255 broadcasthost",
			want:  nil,
		},
		{
			name:  "non-sink address skipped",
			input: "192.168.1.1 router.lan",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseAll(t, tt.input))
		})
	}
}

func TestParse_PlainDomains(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Rule
	}{
		{
			name:  "bare domain covers subdomains",
			input: "ads.example.com",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "wildcard prefix stripped",
			input: "*.example.com",
			want:  []Rule{{Domain: "example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "trailing dot stripped",
			input: "ads.example.com.",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "uppercase normalized",
			input: "Ads.EXAMPLE.com",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "underscore label accepted",
			input: "_dmarc.example.com",
			want:  []Rule{{Domain: "_dmarc.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "single label skipped",
			input: "internal",
			want:  nil,
		},
		{
			name:  "leading dot form covers subdomains",
			input: ".example.com^",
			want:  []Rule{{Domain: "example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "leading dot without separator",
			input: ".example.com",
			want:  []Rule{{Domain: "example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "bare ipv4 skipped",
			input: "192.168.1.1",
			want:  nil,
		},
		{
			name:  "mid-label wildcard kept for regex conversion",
			input: "ads*.example.com",
			want:  []Rule{{Domain: "ads*.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "wildcard-only rule skipped",
			input: "*.*",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseAll(t, tt.input))
		})
	}
}

func TestParse_Adblock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Rule
	}{
		{
			name:  "domain anchor",
			input: "||ads.example.com^",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "without separator",
			input: "||ads.example.com",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "trailing pipe",
			input: "||ads.example.com^|",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "exception rule allows",
			input: "@@||cdn.example.com^",
			want:  []Rule{{Domain: "cdn.example.com", Action: Allow, Subdomains: true}},
		},
		{
			name:  "safe modifier kept",
			input: "||ads.example.com^$important",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "context modifier skipped",
			input: "||ads.example.com^$third-party",
			want:  nil,
		},
		{
			name:  "dnsrewrite to nxdomain blocks",
			input: "||mask.icloud.com^$dnsrewrite=NXDOMAIN",
			want:  []Rule{{Domain: "mask.icloud.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "dnsrewrite to refused blocks",
			input: "||ads.example.com^$dnsrewrite=REFUSED",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "dnsrewrite to an unroutable address blocks",
			input: "||ads.example.com^$dnsrewrite=NOERROR;A;0.0.0.0",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "dnsrewrite to a real address skipped",
			input: "||ads.example.com^$dnsrewrite=NOERROR;A;192.0.2.1",
			want:  nil,
		},
		{
			name:  "dnsrewrite to a cname skipped",
			input: "||ads.example.com^$dnsrewrite=example.net",
			want:  nil,
		},
		{
			name:  "single label kept in adblock syntax",
			input: "||zip^",
			want:  []Rule{{Domain: "zip", Action: Block, Subdomains: true}},
		},
		{
			name:  "path rule skipped",
			input: "||example.com/ads/banner.png",
			want:  nil,
		},
		{
			name:  "url anchor skipped",
			input: "|http://ads.example.com^",
			want:  nil,
		},
		{
			name:  "cosmetic filter skipped",
			input: "example.com##.ad-banner",
			want:  nil,
		},
		{
			name:  "cosmetic exception skipped",
			input: "example.com#@#.ad-banner",
			want:  nil,
		},
		{
			name:  "extended cosmetic skipped",
			input: "example.com#?#div:has(> .ad)",
			want:  nil,
		},
		{
			name:  "mid-label wildcard kept for regex conversion",
			input: "||ads-backup-*.example.com^",
			want:  []Rule{{Domain: "ads-backup-*.example.com", Action: Block, Subdomains: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseAll(t, tt.input))
		})
	}
}

func TestParse_Dnsmasq(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Rule
	}{
		{
			name:  "address to sink",
			input: "address=/ads.example.com/0.0.0.0",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "address to nxdomain",
			input: "address=/ads.example.com/#",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "local form",
			input: "local=/ads.example.com/",
			want:  []Rule{{Domain: "ads.example.com", Action: Block, Subdomains: true}},
		},
		{
			name:  "several domains",
			input: "address=/a.example.com/b.example.com/0.0.0.0",
			want: []Rule{
				{Domain: "a.example.com", Action: Block, Subdomains: true},
				{Domain: "b.example.com", Action: Block, Subdomains: true},
			},
		},
		{
			name:  "redirect to real address skipped",
			input: "address=/ads.example.com/192.168.1.5",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseAll(t, tt.input))
		})
	}
}

func TestParse_SkippedLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "blank", input: "   \t  "},
		{name: "hash comment", input: "# this is a comment"},
		{name: "bang comment", input: "! Title: Example List"},
		{name: "adblock header", input: "[Adblock Plus 2.0]"},
		{name: "regex rule", input: "/ads?[0-9]\\.example\\.com/"},
		{name: "empty label", input: "ads..example.com"},
		{name: "invalid character", input: "ads!example.com"},
		{name: "label too long", input: strings.Repeat("a", 64) + ".example.com"},
		{name: "domain too long", input: strings.Repeat("a.", 130) + "example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Empty(t, parseAll(t, tt.input))
		})
	}
}

func TestParse_MixedFormatList(t *testing.T) {
	input := strings.Join([]string{
		"! Title: Mixed list",
		"# comment",
		"",
		"0.0.0.0 hosts.example.com",
		"plain.example.com",
		"||adblock.example.com^",
		"@@||allowed.example.com^",
		"address=/dnsmasq.example.com/0.0.0.0",
		"example.com##.banner",
		"127.0.0.1 localhost",
	}, "\n")

	assert.Equal(t, []Rule{
		{Domain: "hosts.example.com", Action: Block},
		{Domain: "plain.example.com", Action: Block, Subdomains: true},
		{Domain: "adblock.example.com", Action: Block, Subdomains: true},
		{Domain: "allowed.example.com", Action: Allow, Subdomains: true},
		{Domain: "dnsmasq.example.com", Action: Block, Subdomains: true},
	}, parseAll(t, input))
}

func TestParse_StopsOnBreak(t *testing.T) {
	input := "a.example.com\nb.example.com\nc.example.com"

	var count int
	for range Parse(strings.NewReader(input)) {
		count++
		break
	}
	assert.Equal(t, 1, count)
}

func TestParse_LineTooLong(t *testing.T) {
	input := strings.Repeat("a", maxLineSize+1)

	var gotErr error
	for _, err := range Parse(strings.NewReader(input)) {
		if err != nil {
			gotErr = err
		}
	}
	assert.ErrorContains(t, gotErr, "token too long")
}

func TestScan(t *testing.T) {
	tests := map[string]struct {
		input    string
		rules    int
		attempts int
	}{
		"comments and blanks count nowhere": {
			input:    "! header\n# comment\n\n   \nads.example.com\n",
			rules:    1,
			attempts: 1,
		},
		"a hosts line offers every domain on it": {
			input:    "0.0.0.0 a.example.com b.example.com c.example.com\n",
			rules:    3,
			attempts: 3,
		},
		"failures on a hosts line are not masked by its successes": {
			input:    "0.0.0.0 a.example.com not!a!domain also!bad\n",
			rules:    1,
			attempts: 3,
		},
		"a line that is not a rule still counts as an attempt": {
			input:    "not found, visit some.domain.com\nexample.com##.ad\n",
			rules:    0,
			attempts: 2,
		},
		"prose offers tokens and yields none of them": {
			input:    "service\ntemporarily\nunavailable\n",
			rules:    0,
			attempts: 3,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rules, attempts, err := Scan(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.rules, rules, "rules")
			assert.Equal(t, tt.attempts, attempts, "attempts")
		})
	}
}

func TestScan_SmallAdGuardList(t *testing.T) {
	const list = `! Title: HaGeZi's Apple Private Relay/DNS Bypass
! Description: Blocks Apple Private Relay and possible Apple DNS bypasses correctly.
! Syntax: AdGuard Home/DNS
! 
||mask.icloud.com^$dnsrewrite=NXDOMAIN
||mask-h2.icloud.com^$dnsrewrite=NXDOMAIN
||mask-canary.icloud.com^$dnsrewrite=NXDOMAIN
||doh.dns.apple.com^$dnsrewrite=NXDOMAIN
||doh-dns-apple-com.v.aaplimg.com^$dnsrewrite=NXDOMAIN
||doh.dns.apple.com.v.aaplimg.com^$dnsrewrite=NXDOMAIN
||_dns.resolver.arpa^$dnsrewrite=NXDOMAIN
`
	rules, attempts, err := Scan(strings.NewReader(list))
	require.NoError(t, err)
	assert.Equal(t, 7, rules)
	assert.Equal(t, 7, attempts)
	assert.GreaterOrEqual(t, float64(rules)/float64(attempts), minListValidRulesFraction)
}

func TestLineScanner_SplitsLikeBufioScanner(t *testing.T) {
	inputs := map[string]string{
		"trailing newline":    "0.0.0.0 a.example.com\n||b.example.com^\n",
		"no trailing newline": "0.0.0.0 a.example.com\n||b.example.com^",
		"crlf":                "0.0.0.0 a.example.com\r\n||b.example.com^\r\n",
		"blank lines":         "\n\n0.0.0.0 a.example.com\n\n\n",
		"empty":               "",
		"only newlines":       "\n\n\n",
		"comments":            "! header\n# note\r\nads.example.com\n",
		"prose":               "service\ntemporarily\nunavailable",
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			wantRules, wantAttempts := scanWithBufio(input)
			for _, size := range []int{1, 2, 3, 7, 64, len(input) + 1} {
				rules, attempts, err := Scan(iotest.OneByteReader(strings.NewReader(input)))
				require.NoError(t, err)
				assert.Equal(t, wantRules, rules, "rules")
				assert.Equal(t, wantAttempts, attempts, "attempts")

				var sc lineScanner
				for chunk := range chunks(input, size) {
					_, err := sc.Write([]byte(chunk))
					require.NoError(t, err)
				}
				sc.Flush()
				require.NoError(t, sc.err)
				assert.Equal(t, wantRules, sc.rules, "rules, written in %d-byte chunks", size)
				assert.Equal(t, wantAttempts, sc.attempts, "attempts, written in %d-byte chunks", size)
			}
		})
	}
}

func TestLineScanner_LineTooLong(t *testing.T) {
	_, _, err := Scan(strings.NewReader(strings.Repeat("a", maxLineSize+1)))
	assert.ErrorIs(t, err, bufio.ErrTooLong)
}

func chunks(s string, size int) iter.Seq[string] {
	return func(yield func(string) bool) {
		for len(s) > 0 {
			n := min(size, len(s))
			if !yield(s[:n]) {
				return
			}
			s = s[n:]
		}
	}
}

func scanWithBufio(input string) (rules, attempts int) {
	sc := bufio.NewScanner(strings.NewReader(input))
	sc.Buffer(make([]byte, 0, 4096), maxLineSize)
	var buf []Rule
	for sc.Scan() {
		var n int
		buf, n = parseLine(sc.Text(), buf[:0])
		rules += len(buf)
		attempts += n
	}
	return rules, attempts
}
