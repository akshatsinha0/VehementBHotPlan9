package p9

const (
	Tversion = 100
	Rversion = 101
	Tauth    = 102
	Rauth    = 103
	Tattach  = 104
	Rattach  = 105
	Terror   = 106
	Rerror   = 107
	Tflush   = 108
	Rflush   = 109
	Twalk    = 110
	Rwalk    = 111
	Topen    = 112
	Ropen    = 113
	Tcreate  = 114
	Rcreate  = 115
	Tread    = 116
	Rread    = 117
	Twrite   = 118
	Rwrite   = 119
	Tclunk   = 120
	Rclunk   = 121
	Tremove  = 122
	Rremove  = 123
	Tstat    = 124
	Rstat    = 125
	Twstat   = 126
	Rwstat   = 127
)

const (
	QTDIR  = 0x80
	QTFILE = 0x00
)

type Qid struct {
	Type uint8
	Vers uint32
	Path uint64
}

type Dir struct {
	Type   uint16
	Dev    uint32
	Qid    Qid
	Mode   uint32
	Atime  uint32
	Mtime  uint32
	Length uint64
	Name   string
	Uid    string
	Gid    string
	Muid   string
	Ext    string
	NUid   uint32
	NGid   uint32
	NMuid  uint32
}

type Fcall struct {
	Type   uint8
	Tag    uint16
	Fid    uint32
	Newfid uint32
	Mode   uint8
	Iounit uint32
	Qid    Qid
	Version string
	Uname   string
	Aname   string
	Wname   []string
	Wqid    []Qid
	Offset  uint64
	Count   uint32
	Data    []byte
	Stat    []byte
	Errno   uint32
}


//unplanned stir