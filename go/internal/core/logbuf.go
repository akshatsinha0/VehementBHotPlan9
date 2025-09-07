package core

import (
	"sync/atomic"
)

type Ring struct {
	buf []byte
	mask uint64
	w atomic.Uint64
}

func NewRing(powerOfTwo int) *Ring {
	n := 1
	for n < powerOfTwo { n <<= 1 }
	return &Ring{buf: make([]byte, n), mask: uint64(n-1)}
}

func (r *Ring) Append(p []byte) {
	for _, c := range p {
		i := r.w.Add(1)
		r.buf[(i-1)&r.mask] = c
	}
}

func (r *Ring) Snapshot() []byte {
	w := r.w.Load()
	if w == 0 { return nil }
	if w < uint64(len(r.buf)) {
		return append([]byte(nil), r.buf[:w]...)
	}
	out := make([]byte, len(r.buf))
	start := w & r.mask
	copy(out, r.buf[start:])
	copy(out[len(out)-int(start):], r.buf[:start])
	return out
}