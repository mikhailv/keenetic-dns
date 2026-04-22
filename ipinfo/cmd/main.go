package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/mikhailv/keenetic-dns/ipinfo/internal"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "geolite2.mmdb", "path to MMDB or Parquet file")
	flag.Parse()

	var resolver internal.Resolver
	var err error
	if strings.HasSuffix(*dbPath, ".parquet") {
		resolver, err = internal.NewParquetResolver(*dbPath)
	} else {
		resolver, err = internal.NewMMDBResolver(*dbPath)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer resolver.Close()

	mux := http.NewServeMux()
	srv := internal.NewServer(resolver)
	srv.Register(mux)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
