package pvm

import (
	"embed"
	"flag"
	"mime"
)

var (
	Rpc string
	Rest string
	Listen int
)

//go:embed index.html
var IndexHtml []byte

//go:embed js/* img/* css/*
var StaticContent embed.FS

func init() {
	State = &CurrentState{
		PreVotes: make([]*PreVoteMsg, 0),
	}

	flag.StringVar(&Rpc, "rpc", "tcp://127.0.0.1:26657", "Tendermint server's RPC port")
	flag.StringVar(&Rest, "rest", "http://127.0.0.1:1317", "Tendermint server's Rest endpoint")
	flag.IntVar(&Listen, "p", 8080, "HTTP port to listen on")
	flag.Parse()

	_ = mime.AddExtensionType(".html", "text/html; charset=UTF-8")
	_ = mime.AddExtensionType(".js", "application/javascript")
	_ = mime.AddExtensionType(".css", "text/css")
}
