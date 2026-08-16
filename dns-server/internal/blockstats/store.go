package blockstats

import "github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"

type Store = chunkstore.Store[Chunk]
