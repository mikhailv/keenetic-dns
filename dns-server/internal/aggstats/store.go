package aggstats

import "github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"

type Store[K comparable, V any] = chunkstore.Store[Chunk[K, V]]
