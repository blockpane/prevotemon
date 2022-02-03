package main

import (
	"fmt"
	pvm "github.com/blockpane/prevotemon"
	"github.com/gorilla/websocket"
	"github.com/textileio/go-threads/broadcast"
	"log"
	"net/http"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rpc := "tcp://10.254.254.9:26657"
	rest := "http://10.254.254.9:1317"
	listen := 8080

	updates := make(chan []byte)

	go pvm.WatchPrevotes(rpc, rest, updates)

	b := make([]byte, 0)
	var cast broadcast.Broadcaster
	defer cast.Discard()
	var upgrader = websocket.Upgrader{}

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

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		// sockets
		case "/prevote/ws":
			broadcaster(writer, request, &cast)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})

	go func() {
		for {
			b = <-updates
			//fmt.Println(string(b))
			e := cast.Send(b)
			if e != nil {
				_ = log.Output(2, e.Error())
			}
		}
	}()

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", listen), nil))
}
