package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/mikhailv/keenetic-dns/dns-server/web"
	"github.com/mikhailv/keenetic-dns/internal/stream"
)

type requestFilterFactory[T any] func(r *http.Request, q url.Values) FilterFunc[T]

func createListHandler[T any](st *stream.Buffered[T], filterFactory requestFilterFactory[T]) errorHandler {
	return func(w http.ResponseWriter, req *http.Request) (int, error) {
		query := req.URL.Query()
		filter := filterFactory(req, query)

		backward := queryParamSet(query, "backward")

		afterMode := true
		cursor := stream.Cursor(0)
		switch {
		case query.Has("after"):
			cursor, _ = stream.ParseCursor(query.Get("after"))
		case query.Has("before"):
			cursor, _ = stream.ParseCursor(query.Get("before"))
			afterMode = false
		case backward:
			cursor = math.MaxUint64
		}

		count := 50
		if query.Has("count") {
			count, _ = strconv.Atoi(query.Get("count"))
			count = max(1, count)
		}

		res := struct {
			stream.QueryResult[T]
			PrevPageURL string `json:"prev_page_url"`
			NextPageURL string `json:"next_page_url"`
		}{}

		//             after     before
		// forward     -         back+reverse
		// backward    back      reverse

		if backward == afterMode {
			res.QueryResult = st.QueryBackward(cursor, count, filter)
		} else {
			res.QueryResult = st.Query(cursor, count, filter)
		}
		if !afterMode {
			res.Reverse()
		}

		res.PrevPageURL = updateURLQuery(*req.URL, map[string]string{"after": "\x00", "before": fmt.Sprint(res.FirstCursor)})
		res.NextPageURL = updateURLQuery(*req.URL, map[string]string{"after": fmt.Sprint(res.LastCursor), "before": "\x00"})

		if res.Items == nil {
			res.Items = []T{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return http.StatusOK, nil
	}
}

func createStreamHandler[T any](st *stream.Buffered[T], logger *slog.Logger, filterFactory requestFilterFactory[T]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		filter := filterFactory(req, query)

		conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			logger.Error("failed to accept websocket connection", "err", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()

		logger.Debug("accept websocket connection", "client", req.RemoteAddr)
		ctx := conn.CloseRead(req.Context())

		preloadCount := 1
		if query.Has("preload_count") {
			preloadCount, _ = strconv.Atoi(query.Get("preload_count"))
			preloadCount = max(1, preloadCount)
		}

		cursor, _ := stream.ParseCursor(query.Get("cursor"))
		if cursor == 0 {
			cursor = st.QueryBackward(math.MaxUint64, preloadCount+1, nil).LastCursor
		}

		updateCh := make(chan struct{})
		debouncedUpdateCh := debounceUpdateChannel(ctx, time.Second/2, time.Second*2, updateCh)

		stopListen := st.Listen(func(stream.Cursor, T) {
			select {
			case <-ctx.Done():
			case updateCh <- struct{}{}: // signal about update
			default: // do not block
			}
		})
		defer stopListen()

		processNextDataChunk := func() {
			res := st.Query(cursor, 1000, filter)
			if len(res.Items) > 0 {
				if err := wsjson.Write(ctx, conn, res.Items); err != nil {
					logger.Error("failed to send data", "err", err, "cursor", cursor)
				} else {
					cursor = res.LastCursor
				}
			}
		}

		// send initial data chunk without delay
		processNextDataChunk()

		for {
			select {
			case <-ctx.Done():
				logger.Debug("websocket connection closed", "err", ctx.Err())
				return
			case <-debouncedUpdateCh:
				processNextDataChunk()
			}
		}
	})
}

func webBuildFileHandler(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.BuildFS, path)
	}
}

func webBuildDirectoryHandler() http.Handler {
	return http.FileServer(http.FS(web.BuildFS))
}
