package main
import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

/*
#cgo LDFLAGS: -L../../rust/metricslib/target/release -lmetricslib
#include <stdint.h>
char* metrics_dump_counters();
char* metrics_dump_gauges();
void metrics_inc_counter(const char* k, int64_t v);
void metrics_set_gauge(const char* k, int64_t v);
void metrics_reset_counter(const char* k);
void metrics_reset_gauge(const char* k);
void metrics_free(char* p);
*/
import "C"

const (
	msgTversion = 100
	msgRversion = 101
	msgTattach  = 104
	msgRattach  = 105
	msgTwalk    = 110
	msgRwalk    = 111
	msgTopen    = 112
	msgRopen    = 113
	msgTcreate  = 114
	msgRcreate  = 115
	msgTread    = 116
	msgRread    = 117
	msgTwrite   = 118
	msgRwrite   = 119
	msgTclunk   = 120
	msgRclunk   = 121
	msgTstat    = 124
	msgRstat    = 125
)

type qid struct {
	Type  uint8
	Vers  uint32
	Path  uint64
}

type nodeKind int

const (
	kindDir nodeKind = iota
	kindFile
)

type node struct {
	qid  qid
	name string
	kind nodeKind
	parent *node
	children map[string]*node
}

type fidState struct {
	n *node
	omode uint8
}

type server struct {
	msize uint32
	root  *node
	fids  map[uint32]*fidState
	mu    sync.Mutex

	appLog *ring
	sysLog *ring

}

type ring struct {
	buf  []byte
	off  int
	size int
	mu   sync.Mutex
}

func newRing(n int) *ring {
	return &ring{buf: make([]byte, n), size: n}
}

func (r *ring) append(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range p {
		r.buf[r.off%r.size] = b
		r.off++
	}
}

func (r *ring) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	start := 0
	if r.off > r.size {
		start = r.off % r.size
	}
	if r.off < r.size {
		return append([]byte(nil), r.buf[:r.off]...)
	}
	out := make([]byte, r.size)
	copy(out, r.buf[start:])
	copy(out[r.size-start:], r.buf[:start])
	return out
}

func newServer() *server {
	root := &node{name: "", kind: kindDir, children: make(map[string]*node)}
	svc := &node{name: "svc", kind: kindDir, parent: root, children: make(map[string]*node)}
	root.children["svc"] = svc
	metrics := &node{name: "metrics", kind: kindDir, parent: svc, children: make(map[string]*node)}
	logs := &node{name: "logs", kind: kindDir, parent: svc, children: make(map[string]*node)}
	svc.children["metrics"] = metrics
	svc.children["logs"] = logs
	counters := &node{name: "counters", kind: kindFile, parent: metrics}
	gauges := &node{name: "gauges", kind: kindFile, parent: metrics}
	app := &node{name: "app", kind: kindFile, parent: logs}
	sys := &node{name: "sys", kind: kindFile, parent: logs}
	metrics.children["counters"] = counters
	metrics.children["gauges"] = gauges
	logs.children["app"] = app
	logs.children["sys"] = sys
	assignQids(root, 1)
	return &server{
		msize: 65536,
		root:  root,
		fids:  make(map[uint32]*fidState),
		appLog: newRing(1<<20),
		sysLog: newRing(1<<20),
	}
}

func assignQids(n *node, path uint64) uint64 {
	n.qid = qid{Type: qtype(n), Vers: 0, Path: path}
	next := path + 1
	if n.kind == kindDir {
		for _, ch := range n.children {
			next = assignQids(ch, next)
		}
	}
	return next
}

func qtype(n *node) uint8 {
	if n.kind == kindDir {
		return 0x80
	}
	return 0x00
}

func (s *server) serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(c)
	}
}

func readStr(r io.Reader) (string, error) {
	var n uint16
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return "", err
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeStr(w io.Writer, s string) error {
	if len(s) > 0xFFFF {
		s = s[:0xFFFF]
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(len(s))); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

func (s *server) lookup(p []string) (*node, error) {
	n := s.root
	for _, elem := range p {
		if elem == "" {
			continue
		}
		ch, ok := n.children[elem]
		if !ok {
			return nil, errors.New("enoent")
		}
		n = ch
	}
	return n, nil
}

func (s *server) handleConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	bw := bufio.NewWriter(conn)
	for {
		var sz uint32
		if err := binary.Read(br, binary.LittleEndian, &sz); err != nil {
			return
		}
		hdr := make([]byte, sz-4)
		if _, err := io.ReadFull(br, hdr); err != nil {
			return
		}
		typ := hdr
		tag := binary.LittleEndian.Uint16(hdr[1:3])
		resp := s.dispatch(typ, tag, hdr[3:])
		if resp == nil {
			return
		}
		binary.Write(bw, binary.LittleEndian, uint32(len(resp)+4))
		bw.Write(resp)
		bw.Flush()
	}
}

func rerror(tag uint16, msg string) []byte {
	b := make([]byte, 0, 7+2+len(msg)+2)
	b = append(b, byte(107))
	b = append(b, byte(tag), byte(tag>>8))
	tmp := &strings.Builder{}
	bw := bufio.NewWriter(tmp)
	writeStr(bw, msg)
	bw.Flush()
	b = append(b, tmp.String()...)
	b = append(b, 0, 0)
	return b
}

func (s *server) dispatch(typ byte, tag uint16, body []byte) []byte {
	switch typ {
	case msgTversion:
		return s.rversion(tag)
	case msgTattach:
		return s.rattach(tag, body)
	case msgTwalk:
		return s.rwalk(tag, body)
	case msgTopen:
		return s.ropen(tag, body)
	case msgTread:
		return s.rread(tag, body)
	case msgTwrite:
		return s.rwrite(tag, body)
	case msgTclunk:
		return s.rclunk(tag, body)
	default:
		return rerror(tag, "unsupported")
	}
}

func (s *server) rversion(tag uint16) []byte {
	var out []byte
	out = append(out, byte(msgRversion))
	out = append(out, byte(tag), byte(tag>>8))
	tmp := &strings.Builder{}
	bw := bufio.NewWriter(tmp)
	binary.Write(bw, binary.LittleEndian, uint32(s.msize))
	writeStr(bw, "9P2000.u")
	bw.Flush()
	out = append(out, tmp.String()...)
	return out
}

func (s *server) rattach(tag uint16, body []byte) []byte {
	if len(body) < 4+4 {
		return rerror(tag, "badattach")
	}
	fid := binary.LittleEndian.Uint32(body[0:4])
	// afid ignored
	un := ""
	an := ""
	off := 8
	{
		r := bytesReader(body[off:])
		us, _ := readStr(&r)
		off += 2 + len(us)
		as, _ := readStr(&r)
		un = us
		an = as
	}
	_ = un
	_ = an
	s.mu.Lock()
	s.fids[fid] = &fidState{n: s.root}
	s.mu.Unlock()
	var out []byte
	out = append(out, byte(msgRattach))
	out = append(out, byte(tag), byte(tag>>8))
	q := s.root.qid
	out = append(out, q.Type)
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint32(buf[0:4], q.Vers)
	binary.LittleEndian.PutUint64(buf[4:12], q.Path)
	out = append(out, buf...)
	return out
}

type bytesReader []byte

func (b *bytesReader) Read(p []byte) (int, error) {
	n := copy(p, *b)
	*b = (*b)[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (s *server) rwalk(tag uint16, body []byte) []byte {
	if len(body) < 4+4+2 {
		return rerror(tag, "badwalk")
	}
	fid := binary.LittleEndian.Uint32(body[0:4])
	newfid := binary.LittleEndian.Uint32(body[4:8])
	nw := binary.LittleEndian.Uint16(body[8:10])
	r := bytesReader(body[10:])
	names := make([]string, 0, nw)
	for i := 0; i < int(nw); i++ {
		ns, _ := readStr(&r)
		names = append(names, ns)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.fids[fid]
	if !ok {
		return rerror(tag, "badfid")
	}
	tgt, err := s.lookup(namesFrom(cur.n, names))
	if err != nil {
		return rerror(tag, "enoent")
	}
	s.fids[newfid] = &fidState{n: tgt}
	var out []byte
	out = append(out, byte(msgRwalk))
	out = append(out, byte(tag), byte(tag>>8))
	out = append(out, 1, 0)
	q := tgt.qid
	out = append(out, q.Type)
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint32(buf[0:4], q.Vers)
	binary.LittleEndian.PutUint64(buf[4:12], q.Path)
	out = append(out, buf...)
	return out
}

func namesFrom(start *node, add []string) []string {
	var p []string
	for n := start; n != nil && n.parent != nil; n = n.parent {
		p = append([]string{n.name}, p...)
	}
	p = append(p, add...)
	return p
}

func (s *server) ropen(tag uint16, body []byte) []byte {
	if len(body) < 4+1 {
		return rerror(tag, "badopen")
	}
	fid := binary.LittleEndian.Uint32(body[0:4])
	mode := body[10]
	s.mu.Lock()
	st, ok := s.fids[fid]
	if ok {
		st.omode = mode
	}
	s.mu.Unlock()
	if !ok {
		return rerror(tag, "badfid")
	}
	var out []byte
	out = append(out, byte(msgRopen))
	out = append(out, byte(tag), byte(tag>>8))
	q := st.n.qid
	out = append(out, q.Type)
	buf := make([]byte, 12+4)
	binary.LittleEndian.PutUint32(buf[0:4], q.Vers)
	binary.LittleEndian.PutUint64(buf[4:12], q.Path)
	binary.LittleEndian.PutUint32(buf[12:16], 65536)
	out = append(out, buf...)
	return out
}

func (s *server) rread(tag uint16, body []byte) []byte {
	if len(body) < 4+8+4 {
		return rerror(tag, "badread")
	}
	fid := binary.LittleEndian.Uint32(body[0:4])
	off := binary.LittleEndian.Uint64(body[4:12])
	cnt := binary.LittleEndian.Uint32(body[12:16])
	s.mu.Lock()
	st, ok := s.fids[fid]
	s.mu.Unlock()
	if !ok {
		return rerror(tag, "badfid")
	}
	data := s.render(st.n)
	if off > uint64(len(data)) {
		data = []byte{}
	} else {
		data = data[off:]
	}
	if uint32(len(data)) > cnt {
		data = data[:cnt]
	}
	var out []byte
	out = append(out, byte(msgRread))
	out = append(out, byte(tag), byte(tag>>8))
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(data)))
	out = append(out, buf...)
	out = append(out, data...)
	return out
}

func (s *server) rwrite(tag uint16, body []byte) []byte {
	if len(body) < 4+8+4 {
		return rerror(tag, "badwrite")
	}
	fid := binary.LittleEndian.Uint32(body[0:4])
	off := binary.LittleEndian.Uint64(body[4:12])
	_ = off
	n := binary.LittleEndian.Uint32(body[12:16])
	data := body[16 : 16+int(n)]
	s.mu.Lock()
	st, ok := s.fids[fid]
	s.mu.Unlock()
	if !ok {
		return rerror(tag, "badfid")
	}
	err := s.apply(st.n, data)
	if err != nil {
		return rerror(tag, err.Error())
	}
	var out []byte
	out = append(out, byte(msgRwrite))
	out = append(out, byte(tag), byte(tag>>8))
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(data)))
	out = append(out, buf...)
	return out
}

func (s *server) rclunk(tag uint16, body []byte) []byte {
	if len(body) < 4 {
		return rerror(tag, "badclunk")
	}
	fid := binary.LittleEndian.Uint32(body[0:4])
	s.mu.Lock()
	delete(s.fids, fid)
	s.mu.Unlock()
	var out []byte
	out = append(out, byte(msgRclunk))
	out = append(out, byte(tag), byte(tag>>8))
	return out
}

func (s *server) render(n *node) []byte {
	switch {
	case n == s.root || n.kind == kindDir:
		var b strings.Builder
		for name := range n.children {
			b.WriteString(name)
			b.WriteByte('\n')
		}
		return []byte(b.String())
	}
	if n.name == "counters" {
		p := C.metrics_dump_counters()
		defer C.metrics_free(p)
		return []byte(C.GoString(p))
	}
	if n.name == "gauges" {
		p := C.metrics_dump_gauges()
		defer C.metrics_free(p)
		return []byte(C.GoString(p))
	}
	if n.name == "app" {
		return s.appLog.snapshot()
	}
	if n.name == "sys" {
		return s.sysLog.snapshot()
	}
	return []byte{}
}

func (s *server) apply(n *node, data []byte) error {
	line := strings.TrimSpace(string(data))
	if n.name == "counters" {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts == "reset" {
			cs := C.CString(parts[11])
			C.metrics_reset_counter(cs)
			C.free(unsafe.Pointer(cs))
			return nil
		}
		if len(parts) == 2 && strings.HasPrefix(parts[11], "+") {
			k := C.CString(parts)
			v, _ := strconv.ParseInt(parts[11][1:], 10, 64)
			C.metrics_inc_counter(k, C.longlong(v))
			C.free(unsafe.Pointer(k))
			return nil
		}
		return errors.New("usage: key +N | reset key")
	}
	if n.name == "gauges" {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts == "reset" {
			cs := C.CString(parts[11])
			C.metrics_reset_gauge(cs)
			C.free(unsafe.Pointer(cs))
			return nil
		}
		if len(parts) == 2 {
			k := C.CString(parts)
			v, _ := strconv.ParseInt(parts[11], 10, 64)
			C.metrics_set_gauge(k, C.longlong(v))
			C.free(unsafe.Pointer(k))
			return nil
		}
		return errors.New("usage: key N | reset key")
	}
	if n.name == "app" {
		s.appLog.append(append([]byte(line), '\n'))
		return nil
	}
	if n.name == "sys" {
		s.sysLog.append(append([]byte(line), '\n'))
		return nil
	}
	return errors.New("readonly")
}

func main() {
	addr := ":1564"
	if v := os.Getenv("NINEP_ADDR"); v != "" {
		addr = v
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	s := newServer()
	if err := s.serve(ln); err != nil {
		panic(err)
	}
}
