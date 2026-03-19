package storage

import (
	"bufio"
	"errors"
	"io"
	"iter"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/gzip"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type DNSStore struct {
	index *LookupIndex
}

func NewDNSStore(extraTTL time.Duration) *DNSStore {
	return &DNSStore{
		index: NewLookupIndex(extraTTL),
	}
}

func (s *DNSStore) LookupIP(ip types.IPv4) []*types.DomainLookup {
	return s.index.LookupIP(ip)
}

func (s *DNSStore) Add(rec *types.DomainLookup) {
	s.index.Add(rec)
}

func (s *DNSStore) Remove(rec *types.DomainLookup) {
	s.index.Remove(rec)
}

func (s *DNSStore) RemoveExpired() []*types.DomainLookup {
	return s.index.RemoveExpired()
}

func (s *DNSStore) Iterator() iter.Seq[*types.DomainLookup] {
	return s.index.Iterator()
}

func (s *DNSStore) Load(reader io.Reader) (count int, loadErr error) {
	bufReader := bufio.NewReader(reader)
	gz, err := gzip.NewReader(bufReader)
	if err != nil {
		return 0, err
	}
	defer handleError(gz.Close, &err)

	s.index.Clear()

	decoder := cbor.NewDecoder(gz)

	for {
		var r types.DomainLookup
		if err := decoder.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, err
		}
		if s.index.Add(&r) {
			count++
		}
	}
	return count, nil
}

func (s *DNSStore) Save(writer io.Writer) (count int, err error) {
	bufWriter := bufio.NewWriter(writer)
	defer handleError(bufWriter.Flush, &err)
	gz := gzip.NewWriter(bufWriter)
	defer handleError(gz.Close, &err)

	encoder := cbor.NewEncoder(gz)
	for rec := range s.index.Iterator() {
		if err := encoder.Encode(rec); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func handleError(fn func() error, err *error) {
	fnErr := fn()
	if *err == nil {
		*err = fnErr
	}
}
