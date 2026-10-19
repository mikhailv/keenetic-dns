package conntrack

import (
	"context"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"
)

type Store interface {
	chunkstore.Store[Chunk]
	LoadLast(ctx context.Context) (chunk Chunk, ok bool, err error)
}
