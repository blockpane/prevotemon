package main

import (
	"fmt"
	pvm "github.com/blockpane/prevotemon"
	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/json"
	"github.com/textileio/go-threads/broadcast"
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var vCast broadcast.Broadcaster
	defer vCast.Discard()

	var rCast broadcast.Broadcaster
	defer rCast.Discard()

	var pCast broadcast.Broadcaster
	defer pCast.Discard()

	var upgrader = websocket.Upgrader{}
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	broadcaster := func(writer http.ResponseWriter, request *http.Request, b *broadcast.Broadcaster) {
		c, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		defer c.Close()
		sub := b.Listen()
		defer sub.Discard()
		for message := range sub.Channel() {
			_ = c.WriteMessage(websocket.TextMessage, message.([]byte))
		}
	}

	http.Handle("/js/", http.FileServer(http.FS(pvm.StaticContent)))
	http.Handle("/img/", &CacheHandler{})
	http.Handle("/css/", http.FileServer(http.FS(pvm.StaticContent)))

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		// sockets
		case "/rounds/ws":
			broadcaster(writer, request, &rCast)
		case "/prevote/ws":
			broadcaster(writer, request, &vCast)
		case "/progress/ws":
			broadcaster(writer, request, &pCast)
		default:
			writer.WriteHeader(http.StatusNotFound)
		// static
		case "/", "/index.html":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write(pvm.IndexHtml)
		case "/state":
			writer.Header().Set("Content-Type", "application/json")
			j, _ := json.Marshal(pvm.State)
			_, _ = writer.Write(j)
		case "/chainid":
			_, _ = writer.Write([]byte(fmt.Sprintf(`{"chain_id": "%s"}`, pvm.ChainID)))
		case "/history":
			writer.Header().Set("Content-Type", "application/json")
			keys, ok := request.URL.Query()["height"]
			if !ok {
				_, _ = writer.Write([]byte(pvm.BlockNotFound))
				return
			}
			n, err := strconv.Atoi(keys[0])
			if err != nil {
				_, _ = writer.Write([]byte(pvm.BlockNotFound))
				return
			}
			var j []byte
			if n == 0 {
				j, _ = json.Marshal(pvm.State)
			} else {
				j, err = pvm.FetchRecord(int64(n))
				if err != nil {
					j = []byte(pvm.BlockNotFound)
				}
			}
			_, _ = writer.Write(j)
		}
	})

	updates := make(chan []byte, 1)
	rounds := make(chan []byte, 1)
	progress := make(chan []byte, 1)
	go func() {
		for {
			select {
			case u := <-updates:
				_ = vCast.Send(u)
			case p := <-progress:
				_ = pCast.Send(p)
			case r := <-rounds:
				_ = rCast.Send(r)
			}
		}
	}()
	go func() {
		for {
			pvm.WatchPrevotes(pvm.Rpc, pvm.Rest, rounds, updates, progress)
			log.Println("watch prevote routine exited, will retry in 5s")
			time.Sleep(5 * time.Second)
		}
	}()

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", pvm.Listen), nil))
}

// CacheHandler implements the Handler interface with a very long Cache-Control set on responses
type CacheHandler struct{}

func (ch CacheHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "public, max-age=86400")
	http.FileServer(http.FS(pvm.StaticContent)).ServeHTTP(writer, request)
}
