package core

import (
	"bufio"
	"net"
)

type Append func([]byte)

func StartIngest(addr string, sink Append) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil { return nil, err }
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil { return }
			go func() {
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadBytes('\n')
					if err != nil { c.Close(); return }
					sink(line)
				}
			}()
		}
	}()
	return ln, nil
}