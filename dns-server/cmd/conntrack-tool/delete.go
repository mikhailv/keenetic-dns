package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

const asnQueryLimit = 1000

type listFlag []string

func (f *listFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *listFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func runDelete(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	from := fs.String("from", "", "first day, YYYY-MM-DD (UTC)")
	to := fs.String("to", "", "last day, YYYY-MM-DD (UTC), inclusive")
	api := fs.String("ipinfo-api", "https://ip.kpanic.ru", "ipinfo service used to resolve -asn to networks")
	dryRun := fs.Bool("dry-run", false, "only report what would be deleted")
	var nets, asns listFlag
	fs.Var(&nets, "net", "network in CIDR notation, repeatable")
	fs.Var(&asns, "asn", "AS number, repeatable")
	_ = fs.Parse(args)
	if fs.NArg() != 1 || len(nets)+len(asns) == 0 {
		usage()
	}
	tr, err := parseRange(*from, *to)
	if err != nil {
		return err
	}

	prefixes, err := resolvePrefixes(ctx, *api, nets, asns)
	if err != nil {
		return err
	}
	match := func(e conntrack.BucketEntry) bool {
		return containsIP(prefixes, e.SrcIP) || containsIP(prefixes, e.DstIP)
	}

	files, removed, err := deleteEntries(ctx, openStore(fs.Arg(0)), tr, match, *dryRun)
	if err != nil {
		return err
	}
	verb := "deleted"
	if *dryRun {
		verb = "would delete"
	}
	fmt.Fprintf(os.Stderr, "%s %d entries in %d files\n", verb, removed, files)
	return nil
}

func deleteEntries(
	ctx context.Context,
	store conntrack.Store,
	tr conntrack.TimeRange,
	match func(conntrack.BucketEntry) bool,
	dryRun bool,
) (files, removed int, err error) {
	for chunk, err := range store.Load(ctx, tr) {
		if err != nil {
			return files, removed, err
		}
		n := 0
		for i := range chunk.Buckets {
			if !tr.InRange(chunk.Buckets[i].TimeRange.Start) {
				continue
			}
			entries := chunk.Buckets[i].Entries
			chunk.Buckets[i].Entries = slices.DeleteFunc(entries, match)
			n += len(entries) - len(chunk.Buckets[i].Entries)
		}
		if n == 0 {
			continue
		}
		files++
		removed += n
		fmt.Fprintf(os.Stderr, "%s: %d entries\n", chunk.TimeRange.StartTime().UTC().Format("2006-01-02 15:04"), n)
		if !dryRun {
			if err := store.Save(ctx, chunk); err != nil {
				return files, removed, err
			}
		}
	}
	return files, removed, nil
}

func resolvePrefixes(ctx context.Context, api string, nets, asns []string) ([]netip.Prefix, error) {
	prefixes, err := parsePrefixes(nets)
	if err != nil {
		return nil, err
	}
	for _, asn := range asns {
		asnPrefixes, err := queryASNPrefixes(ctx, api, asn)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "AS%s: %d networks\n", asn, len(asnPrefixes))
		prefixes = append(prefixes, asnPrefixes...)
	}
	return prefixes, nil
}

func parsePrefixes(nets []string) ([]netip.Prefix, error) {
	res := make([]netip.Prefix, 0, len(nets))
	for _, n := range nets {
		p, err := netip.ParsePrefix(n)
		if err != nil {
			return nil, fmt.Errorf("invalid -net %q: %w", n, err)
		}
		res = append(res, p.Masked())
	}
	return res, nil
}

func containsIP(prefixes []netip.Prefix, ip types.IPv4) bool {
	addr := netip.AddrFrom4([4]byte(ip[:4]))
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

type asnQueryResponse struct {
	Items []struct {
		Network string `json:"network"`
	} `json:"items"`
	HasMore bool `json:"has_more"`
}

func queryASNPrefixes(ctx context.Context, api, asn string) ([]netip.Prefix, error) {
	if _, err := strconv.ParseUint(asn, 10, 32); err != nil {
		return nil, fmt.Errorf("invalid -asn %q: %w", asn, err)
	}
	var res []netip.Prefix
	for offset := 0; ; offset += asnQueryLimit {
		page, err := fetchASNPage(ctx, api, asn, offset)
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			p, err := netip.ParsePrefix(it.Network)
			if err != nil {
				return nil, fmt.Errorf("AS%s: invalid network %q: %w", asn, it.Network, err)
			}
			res = append(res, p.Masked())
		}
		if !page.HasMore || len(page.Items) == 0 {
			break
		}
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("AS%s: no networks found", asn)
	}
	return res, nil
}

func fetchASNPage(ctx context.Context, api, asn string, offset int) (page asnQueryResponse, resErr error) {
	q := url.Values{
		"field":  {"asn_number"},
		"query":  {asn},
		"offset": {strconv.Itoa(offset)},
		"limit":  {strconv.Itoa(asnQueryLimit)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(api, "/")+"/ip/query?"+q.Encode(), nil)
	if err != nil {
		return page, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return page, err
	}
	defer func() {
		resErr = errors.Join(resErr, resp.Body.Close())
	}()
	if resp.StatusCode != http.StatusOK {
		return page, errors.New("AS" + asn + ": " + resp.Status)
	}
	err = json.NewDecoder(resp.Body).Decode(&page)
	return page, err
}
