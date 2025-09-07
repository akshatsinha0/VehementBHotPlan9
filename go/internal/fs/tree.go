package fs

import (
	"sync"
	"time"
	"vehementbhotplan9/internal/p9"
)
type Kind int

const (
	Dir Kind = iota
	File
)
type Node struct {
	Name string
	Kind Kind
	Mode uint32
	Qid  p9.Qid
	Parent *Node
	Children map[string]*Node
}

type Tree struct {
	Root *Node
	mu sync.RWMutex
}

func NewTree() *Tree {
	root := &Node{Name: "/", Kind: Dir, Mode: 0x80000000 | 0555, Children: map[string]*Node{}}
	svc := &Node{Name: "svc", Kind: Dir, Mode: 0x80000000 | 0555, Parent: root, Children: map[string]*Node{}}
	metrics := &Node{Name: "metrics", Kind: Dir, Mode: 0x80000000 | 0555, Parent: svc, Children: map[string]*Node{}}
	logs := &Node{Name: "logs", Kind: Dir, Mode: 0x80000000 | 0666, Parent: svc, Children: map[string]*Node{}}
	root.Children["svc"] = svc
	svc.Children["metrics"] = metrics
	svc.Children["logs"] = logs
	metrics.Children["counters"] = &Node{Name: "counters", Kind: File, Mode: 0644, Parent: metrics}
	metrics.Children["gauges"] = &Node{Name: "gauges", Kind: File, Mode: 0644, Parent: metrics}
	logs.Children["app"] = &Node{Name: "app", Kind: File, Mode: 0644, Parent: logs}
	logs.Children["sys"] = &Node{Name: "sys", Kind: File, Mode: 0644, Parent: logs}
	assignQids(root, 1)
	return &Tree{Root: root}
}
func assignQids(n *Node, path uint64) uint64 {
	if n.Kind ==Dir {
		n.Qid = p9.Qid{Type: 0x80, Vers: 0, Path: path}
	} else {
		n.Qid = p9.Qid{Type: 0x00, Vers: 0, Path: path}
	}
	next := path + 1
	for _, ch := range n.Children {
		next = assignQids(ch, next)
	}
	return next
}

func (t *Tree) Lookup(parts []string) (*Node, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cur := t.Root
	for _, s := range parts {
		if s == "" || s == "/" { continue }
		n, ok := cur.Children[s]
		if !ok { return nil, false }
		cur = n
	}
	return cur, true
}

func DirFor(n *Node, size uint64) p9.Dir {
	now := uint32(time.Now().Unix())// -> 
	if n.Kind ==Dir {
		return p9.Dir{
			Type: 0, Dev: 0, Qid: n.Qid, Mode: n.Mode, Atime: now, Mtime: now, Length: 0,
			Name: n.Name, Uid: "root", Gid: "root", Muid: "root", Ext: "", NUid: 0xffffffff, NGid: 0xffffffff, NMuid: 0xffffffff,
		}
	}
	return p9.Dir{
		Type: 0, Dev: 0, Qid: n.Qid, Mode: n.Mode, Atime: now, Mtime: now, Length: size,
		Name: n.Name, Uid: "root", Gid: "root", Muid: "root", Ext: "", NUid: 0xffffffff, NGid: 0xffffffff, NMuid: 0xffffffff,
	}
}