package conntrack

import (
	"strconv"
	"strings"

	"github.com/mikhailv/keenetic-dns/agent/internal/api"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

// Parse parses the content of `conntrack -L` into structured entries.
func Parse(content string) []api.ConntrackEntry {
	lines := strings.Split(content, "\n")
	entries := make([]api.ConntrackEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if entry, ok := parseLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// parseLine parses a single nf_conntrack line.
// Each line has: family, proto_num, proto_name, proto_num2, ttl,
// [state], original_dir key=value pairs, reply_dir key=value pairs,
// flags, and metadata.
func parseLine(line string) (api.ConntrackEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return api.ConntrackEntry{}, false
	}

	var entry api.ConntrackEntry
	entry.Protocol = strings.TrimSpace(fields[0])

	if ttl, err := strconv.Atoi(fields[2]); err == nil {
		entry.Ttl = &ttl
	}

	// TCP has a state field (ESTABLISHED, TIME_WAIT, etc.);
	// UDP/ICMP go straight to key=value pairs.
	if entry.Protocol == "tcp" && !strings.Contains(fields[3], "=") {
		entry.State = &fields[3]
		parseKeyValues(&entry, fields[4:])
	} else {
		parseKeyValues(&entry, fields[3:])
	}

	if entry.SrcIp == "" || entry.DstIp == "" {
		return api.ConntrackEntry{}, false
	}

	return entry, true
}

// parseKeyValues processes the key=value pairs and flags
// from a conntrack line. The line has two direction blocks
// (original + reply); we detect the reply block by seeing
// a second "src=" token.
func parseKeyValues(entry *api.ConntrackEntry, fields []string) {
	seenSrc := false
	reply := false

	for _, f := range fields {
		if f == "[ASSURED]" {
			entry.Assured = util.Ptr(true)
			continue
		}
		if strings.HasPrefix(f, "[") {
			continue
		}

		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}

		if k == "src" {
			if seenSrc {
				reply = true
			}
			seenSrc = true
		}

		applyField(entry, k, v, reply)
	}
}

func applyField(e *api.ConntrackEntry, k, v string, reply bool) {
	switch k {
	case "src":
		if !reply {
			e.SrcIp = v
		}
	case "dst":
		if !reply {
			e.DstIp = v
		}
	case "sport":
		if !reply {
			e.SrcPort = parseUint16(v)
		}
	case "dport":
		if !reply {
			e.DstPort = parseUint16(v)
		}
	case "packets":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			if !reply {
				e.PacketsOrig = n
			} else {
				e.PacketsReply = n
			}
		}
	case "bytes":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			if !reply {
				e.BytesOrig = n
			} else {
				e.BytesReply = n
			}
		}
	case "mark":
		if n, err := strconv.Atoi(v); err == nil {
			e.Mark = &n
		}
	case "id":
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			id := uint32(n)
			e.Id = &id
		}
	}
}

func parseUint16(s string) *uint16 {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return nil
	}
	v := uint16(n)
	return &v
}
