package pvm

import (
	"embed"
	"flag"
	"log"
	"mime"
)

var (
	Rpc                 string
	Rest                string
	Listen              int
	Cache               *LocalCache
	redisUrl, redisPass string
	redisTls            bool
	redisDb             int
)

//go:embed index.html
var IndexHtml []byte

//go:embed js/* img/* css/*
var StaticContent embed.FS

func init() {
	State = &CurrentState{
		PreVotes: make([]*PreVoteMsg, 0),
	}
	Cache = newLocalCache()

	flag.StringVar(&Rpc, "rpc", "tcp://127.0.0.1:26657", "Tendermint server's RPC port")
	flag.StringVar(&Rest, "rest", "http://127.0.0.1:1317", "Tendermint server's Rest endpoint")
	flag.IntVar(&Listen, "p", 8080, "HTTP port to listen on")
	flag.StringVar(&redisUrl, "redis", "127.0.0.1:6379", "redis url for storing historical APR data")
	flag.StringVar(&redisPass, "pass", "", "redis password for storing historical APR data")
	flag.IntVar(&redisDb, "db", 0, "redis DB to use")
	flag.BoolVar(&redisTls, "tls", false, "Enable TLS to redis db")
	flag.Parse()

	if _, err := getRedisClient(); err != nil {
		flag.PrintDefaults()
		log.Fatalln("Could not connect to redis:", err)
	}

	_ = mime.AddExtensionType(".html", "text/html; charset=UTF-8")
	_ = mime.AddExtensionType(".js", "application/javascript")
	_ = mime.AddExtensionType(".css", "text/css")
}
