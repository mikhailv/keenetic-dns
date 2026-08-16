package domainstats

import "github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"

type Store = chunkstore.Store[Chunk]
