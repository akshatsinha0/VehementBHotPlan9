package p9

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"vehementbhotplan9/internal/core"
	"vehementbhotplan9/internal/fs"
)

type Server struct {
	Msize uint32
	Tree  *fs.Tree
	App   *core.Ring
	Sys   *core.Ring
	M     *core.MetricsClient
	mu    sync.Mutex
	fids  map[uint32]*fs.Node
}

func NewServer(t *fs.Tree, app, sys *core.Ring, m *core.MetricsClient) *Server {
	return &Server{Msize: 65536, Tree: t, App: app, Sys: sys, M: m, fids: map[uint32]*fs.Node{}}
}

func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil { return err }
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	for {
		var sz uint32
		if err := binary.Read(br, binary.LittleEndian, &sz); err != nil { return }
		body := make([]byte, sz-4)
		if _, err := io.ReadFull(br, body); err != nil { return }
		typ := body[0]
		tag := binary.LittleEndian.Uint16(body[1:3])
		resp := s.dispatch(typ, tag, body[3:])
		if resp == nil { return }
		binary.Write(bw, binary.LittleEndian, uint32(len(resp)+4))
		bw.Write(resp)
		bw.Flush()
	}
}

func (s *Server) dispatch(typ byte, tag uint16, b []byte) []byte {
	switch typ {
	case Tversion:
		return PackRversion(tag, s.Msize, "9P2000.u")
	case Tattach:
		if len(b) < 8 { return PackRerror(tag, "bad attach", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		s.mu.Lock()
		s.fids[fid] = s.Tree.Root
		s.mu.Unlock()
		return PackRattach(tag, s.Tree.Root.Qid)
	case Twalk:
		if len(b) < 10 { return PackRerror(tag, "bad walk", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		newfid := binary.LittleEndian.Uint32(b[4:8])
		nw := binary.LittleEndian.Uint16(b[8:10])
		r := bytes.NewReader(b[10:])
		names := make([]string, 0, nw)
		for i := 0; i < int(nw); i++ {
			var ln uint16
			binary.Read(r, binary.LittleEndian, &ln)
			sb := make([]byte, ln)
			io.ReadFull(r, sb)
			names = append(names, string(sb))
		}
		s.mu.Lock()
		cur := s.fids[fid]
		s.mu.Unlock()
		if cur == nil { return PackRerror(tag, "bad fid", 0) }
		full := append(pathElems(cur), names...)
		n, ok := s.Tree.Lookup(full)
		if !ok { return PackRerror(tag, "enoent", 0) }
		s.mu.Lock()
		s.fids[newfid] = n
		s.mu.Unlock()
		return PackRwalk(tag, []Qid{n.Qid})
	case Topen:
		if len(b) < 5 { return PackRerror(tag, "bad open", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		s.mu.Lock()
		n := s.fids[fid]
		s.mu.Unlock()
		if n == nil { return PackRerror(tag, "bad fid", 0) }
		return PackRopen(tag, n.Qid, s.Msize)
	case Tread:
		if len(b) < 16 { return PackRerror(tag, "bad read", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		off := binary.LittleEndian.Uint64(b[4:12])
		cnt := binary.LittleEndian.Uint32(b[12:16])
		_ = off
		s.mu.Lock()
		n := s.fids[fid]
		s.mu.Unlock()
		if n == nil { return PackRerror(tag, "bad fid", 0) }
		if n.Kind == fs.Dir {
			data := fs.PackDirOf(n)
			return PackRread(tag, data)
		}
		if n.Name == "counters" {
			snap, _ := s.M.DumpCounters()
			d := []byte(snap)
			if uint64(len(d)) <= off { d = []byte{} } else { d = d[off:] }
			if uint32(len(d)) > cnt { d = d[:cnt] }
			return PackRread(tag, d)
		}
		if n.Name == "gauges" {
			snap, _ := s.M.DumpGauges()
			d := []byte(snap)
			if uint64(len(d)) <= off { d = []byte{} } else { d = d[off:] }
			if uint32(len(d)) > cnt { d = d[:cnt] }
			return PackRread(tag, d)
		}
		if n.Name == "app" {
			d := s.App.Snapshot()
			if uint64(len(d)) <= off { d = []byte{} } else { d = d[off:] }
			if uint32(len(d)) > cnt { d = d[:cnt] }
			return PackRread(tag, d)
		}
		if n.Name == "sys" {
			d := s.Sys.Snapshot()
			if uint64(len(d)) <= off { d = []byte{} } else { d = d[off:] }
			if uint32(len(d)) > cnt { d = d[:cnt] }
			return PackRread(tag, d)
		}
		return PackRread(tag, nil)
	case Twrite:
		if len(b) < 16 { return PackRerror(tag, "bad write", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		_ = binary.LittleEndian.Uint64(b[4:12])
		nbytes := binary.LittleEndian.Uint32(b[12:16])
		data := b[16:16+int(nbytes)]
		s.mu.Lock()
		n := s.fids[fid]
		s.mu.Unlock()
		if n == nil { return PackRerror(tag, "bad fid", 0) }
		if n.Name == "counters" {
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(line, "reset ") {
				k := strings.TrimSpace(strings.TrimPrefix(line, "reset "))
				s.M.ResetCounter(k)
			} else {
				parts := strings.Fields(line)
				if len(parts) == 2 && strings.HasPrefix(parts[1], "+") {
					v := parseI64(parts[1][1:])
					s.M.IncCounter(parts[0], v)
				}
			}
			return PackRwrite(tag, nbytes)
		}
		if n.Name == "gauges" {
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(line, "reset ") {
				k := strings.TrimSpace(strings.TrimPrefix(line, "reset "))
				s.M.ResetGauge(k)
			} else {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					v := parseI64(parts[1])
					s.M.SetGauge(parts[0], v)
				}
			}
			return PackRwrite(tag, nbytes)
		}
		if n.Name == "app" {
			s.App.Append(append([]byte(string(data)), '\n'))
			return PackRwrite(tag, nbytes)
		}
		if n.Name == "sys" {
			s.Sys.Append(append([]byte(string(data)), '\n'))
			return PackRwrite(tag, nbytes)
		}
		return PackRerror(tag, "readonly", 0)
	case Tclunk:
		if len(b) < 4 { return PackRerror(tag, "bad clunk", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		s.mu.Lock()
		delete(s.fids, fid)
		s.mu.Unlock()
		return PackRclunk(tag)
	case Tstat:
		if len(b) < 4 { return PackRerror(tag, "bad stat", 0) }
		fid := binary.LittleEndian.Uint32(b[0:4])
		s.mu.Lock()
		n := s.fids[fid]
		s.mu.Unlock()
		if n == nil { return PackRerror(tag, "bad fid", 0) }
		st := fs.PackDirEntry(n, 0)
		return PackRstat(tag, st)
	default:
		return PackRerror(tag, "unsupported", 0)
	}
}

func pathElems(n *fs.Node) []string {
	var p []string
	for cur := n; cur != nil && cur.Parent != nil; cur = cur.Parent {
		p = append([]string{cur.Name}, p...)
	}
	return p
}

func parseI64(s string) int64 {
	var neg bool
	if len(s) > 0 && s[0] == '-' { neg = true; s = s[1:] }
	var v int64
	for i := 0; i < len(s); i++ { v = v*10 + int64(s[i]-'0') }
	if neg { v = -v }
	return v
}