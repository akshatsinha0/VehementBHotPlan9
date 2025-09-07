package p9

import (
	"bytes"
	"encoding/binary"
)

func pu16(b *bytes.Buffer, v uint16) { binary.Write(b, binary.LittleEndian, v) }
func pu32(b *bytes.Buffer, v uint32) { binary.Write(b, binary.LittleEndian, v) }
func pu64(b *bytes.Buffer, v uint64) { binary.Write(b, binary.LittleEndian, v) }

func pstr(b *bytes.Buffer, s string) {
	if len(s) > 0xFFFF { s = s[:0xFFFF] }
	pu16(b, uint16(len(s)))
	b.WriteString(s)
}

func pqid(b *bytes.Buffer, q Qid) {
	b.WriteByte(q.Type)
	pu32(b, q.Vers)
	pu64(b, q.Path)
}

func PackRerror(tag uint16, emsg string, errno uint32) []byte {
	var body bytes.Buffer
	pstr(&body, emsg)
	pu32(&body, errno)
	return pack(Rerror, tag, body.Bytes())
}

func PackRversion(tag uint16, msize uint32, vers string) []byte {
	var body bytes.Buffer
	pu32(&body, msize)
	pstr(&body, vers)
	return pack(Rversion, tag, body.Bytes())
}

func PackRattach(tag uint16, q Qid) []byte {
	var body bytes.Buffer
	pqid(&body, q)
	return pack(Rattach, tag, body.Bytes())
}

func PackRwalk(tag uint16, qs []Qid) []byte {
	var body bytes.Buffer
	pu16(&body, uint16(len(qs)))
	for _, q := range qs { pqid(&body, q) }
	return pack(Rwalk, tag, body.Bytes())
}

func PackRopen(tag uint16, q Qid, iounit uint32) []byte {
	var body bytes.Buffer
	pqid(&body, q)
	pu32(&body, iounit)
	return pack(Ropen, tag, body.Bytes())
}

func PackRclunk(tag uint16) []byte {
	return pack(Rclunk, tag, nil)
}

func PackRwrite(tag uint16, n uint32) []byte {
	var body bytes.Buffer
	pu32(&body, n)
	return pack(Rwrite, tag, body.Bytes())
}

func PackRread(tag uint16, data []byte) []byte {
	var body bytes.Buffer
	pu32(&body, uint32(len(data)))
	body.Write(data)
	return pack(Rread, tag, body.Bytes())
}

func PackRstat(tag uint16, stat []byte) []byte {
	var body bytes.Buffer
	pu32(&body, uint32(len(stat)))
	body.Write(stat)
	return pack(Rstat, tag, body.Bytes())
}

func pack(typ uint8, tag uint16, body []byte) []byte {
	var out bytes.Buffer
	total := 4 + 1 + 2 + len(body)
	pu32(&out, uint32(total))
	out.WriteByte(typ)
	binary.Write(&out, binary.LittleEndian, tag)
	out.Write(body)
	return out.Bytes()
}

func PackStat(d Dir) []byte {
	var x bytes.Buffer
	pu16(&x, 0)
	pu16(&x, d.Type)
	pu32(&x, d.Dev)
	pqid(&x, d.Qid)
	pu32(&x, d.Mode)
	pu32(&x, d.Atime)
	pu32(&x, d.Mtime)
	pu64(&x, d.Length)
	pstr(&x, d.Name)
	pstr(&x, d.Uid)
	pstr(&x, d.Gid)
	pstr(&x, d.Muid)
	pstr(&x, d.Ext)
	pu32(&x, d.NUid)
	pu32(&x, d.NGid)
	pu32(&x, d.NMuid)
	b := x.Bytes()
	sz := uint16(len(b) - 2)
	binary.LittleEndian.PutUint16(b[0:2], sz)
	return b
}