package fs

import (
	"bytes"
	"vehementbhotplan9/internal/p9"
)

func PackDirEntry(n *Node, size uint64) []byte {
	d := DirFor(n, size)
	return p9.PackStat(d)
}

func PackDirOf(n *Node) []byte {
	if n.Kind != Dir { return nil }
	var b bytes.Buffer
	for _, ch := range n.Children {
		st := PackDirEntry(ch, 0)
		b.Write(st)
	}
	return b.Bytes()
}