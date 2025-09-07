package main

import (
	"net"
	"os"
	"vehementbhotplan9/internal/core"
	"vehementbhotplan9/internal/fs"
	"vehementbhotplan9/internal/p9"
)

func main() {
	app := core.NewRing(1<<20)
	sys := core.NewRing(1<<20)
	tree := fs.NewTree()
	mc, err := core.NewMetricsClient(env("METRICS_ADDR", "127.0.0.1:9630"))
	if err != nil { panic(err) }
	_, err = core.StartIngest(env("INGEST_ADDR", "127.0.0.1:9627"), func(b []byte){ app.Append(b) })
	if err != nil { panic(err) }
	l, err := net.Listen("tcp", env("NINEP_ADDR", "127.0.0.1:1564"))
	if err != nil { panic(err) }
	s := p9.NewServer(tree, app, sys, mc)
	if err := s.Serve(l); err != nil { panic(err) }
}

func env(k, d string) string {
	v := os.Getenv(k)
	if v == "" { return d }
	return v
}