package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type exportRow struct {
	Time         int64  `parquet:"ts,timestamp(millisecond)"`
	Proto        string `parquet:"proto,dict"`
	Src          string `parquet:"src,dict"`
	Dst          string `parquet:"dst,dict"`
	DstPort      int32  `parquet:"dport"`
	BytesOrig    int64  `parquet:"bytes_orig"`
	BytesReply   int64  `parquet:"bytes_reply"`
	PacketsOrig  int64  `parquet:"packets_orig"`
	PacketsReply int64  `parquet:"packets_reply"`
	Conns        int32  `parquet:"conns"`
}

func runExport(ctx context.Context, args []string) (resErr error) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	from := fs.String("from", "", "first day, YYYY-MM-DD (UTC)")
	to := fs.String("to", "", "last day, YYYY-MM-DD (UTC), inclusive")
	out := fs.String("o", "", "output parquet file")
	_ = fs.Parse(args)
	if fs.NArg() != 1 || *out == "" {
		usage()
	}
	tr, err := parseRange(*from, *to)
	if err != nil {
		return err
	}

	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer util.HandleError(f.Close, &resErr)

	w := parquet.NewGenericWriter[exportRow](f, parquet.Compression(&zstd.Codec{Level: zstd.SpeedDefault}))
	chunks, rows := 0, 0
	for chunk, err := range openStore(fs.Arg(0)).Load(ctx, tr) {
		if err != nil {
			return err
		}
		batch := chunkRows(chunk, tr)
		if _, err := w.Write(batch); err != nil {
			return err
		}
		chunks++
		rows += len(batch)
	}
	if err := w.Close(); err != nil {
		return errors.Join(err, os.Remove(*out))
	}
	fmt.Fprintf(os.Stderr, "exported %d rows from %d chunks to %s\n", rows, chunks, *out)
	return nil
}

func chunkRows(chunk conntrack.Chunk, tr conntrack.TimeRange) []exportRow {
	var rows []exportRow
	for _, bucket := range chunk.Buckets {
		if !tr.InRange(bucket.TimeRange.Start) {
			continue
		}
		for _, e := range bucket.Entries {
			rows = append(rows, exportRow{
				Time:         int64(bucket.TimeRange.Start) * 1000,
				Proto:        e.Protocol.String(),
				Src:          e.SrcIP.String(),
				Dst:          e.DstIP.String(),
				DstPort:      int32(e.DstPort),
				BytesOrig:    e.BytesOrig,
				BytesReply:   e.BytesReply,
				PacketsOrig:  int64(e.PacketsOrig),
				PacketsReply: int64(e.PacketsReply),
				Conns:        int32(len(e.ConnIDs)),
			})
		}
	}
	return rows
}
