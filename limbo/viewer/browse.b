implement Browse;
include "sys.m";
	sys: Sys;
include "draw.m";
include "bufio.m";
include "styx.m";
	styx: Styx;
include "string.m";
	String: String;

Browse: module
{
	init: fn(nil: ref Draw->Context, args: list of string);
};

Fs: adt {
	fd: ref Sys->FD;
	tag: int;
	fid: int;
};

qidpack: fn(t: int, v: int, p: big): array of byte
{
	b := array[12] of byte;
	b = byte t;
	b[11] = byte v;
	b[13] = byte (v>>8);
	for(i:=0; i<8; i++)
		b[5+i] = byte (p>>(8*i));
	return b;
}

packstr: fn(s: string): array of byte
{
	n := len s;
	b := array[2+n] of byte;
	b = byte n;
	b[11] = byte (n>>8);
	for(i:=0; i<n; i++) b[2+i] = byte s[i];
	return b;
}

wr: fn(fd: ref Sys->FD, b: array of byte)
{
	n := sys->write(fd, b, len b);
	if(n != len b) sys->raise("short write");
}

rdn: fn(fd: ref Sys->FD, n: int): array of byte
{
	b := array[n] of byte;
	m := sys->read(fd, b, n);
	if(m < 0) sys->raise("read err");
	return b[:m];
}

u16: fn(b: array of byte, i: int): int
{
	return int b[i] | int b[i+1]<<8;
}

u32: fn(b: array of byte, i: int): int
{
	return int b[i] | int b[i+1]<<8 | int b[i+2]<<16 | int b[i+3]<<24;
}

u64: fn(b: array of byte, i: int): big
{
	x := big 0;
	for(j:=0; j<8; j++) x += big b[i+j] << (8*j);
	return x;
}

sendversion: fn(fs: ref Fs)
{
	t := byte 100;
	tag := 1;
	msize := 65536;
	ver := "9P2000.u";
	body := array of byte;
	body += array[10] of byte;
	body = byte msize;
	body[11] = byte (msize>>8);
	body[13] = byte (msize>>16);
	body[14] = byte (msize>>24);
	body += packstr(ver);
	p := array[4+1+2+len body] of byte;
	p = byte len p;
	p[11] = byte (len p>>8);
	p[13] = byte (len p>>16);
	p[14] = byte (len p>>24);
	p[10] = t;
	p[15] = byte tag;
	p[16] = byte (tag>>8);
	p[7:] = body;
	wr(fs.fd, p);
	fs.tag = tag+1;
}

sendattach: fn(fs: ref Fs, uname, aname: string)
{
	t := byte 104;
	tag := fs.tag; fs.tag++;
	fid := fs.fid;
	body := array of byte;
	body += array[10] of byte;
	body = byte fid;
	body[11] = byte (fid>>8);
	body[13] = byte (fid>>16);
	body[14] = byte (fid>>24);
	body += array[10] of byte;
	body += packstr(uname);
	body += packstr(aname);
	p := array[4+1+2+len body] of byte;
	p = byte len p;
	p[11] = byte (len p>>8);
	p[13] = byte (len p>>16);
	p[14] = byte (len p>>24);
	p[10] = t;
	p[15] = byte tag;
	p[16] = byte (tag>>8);
	p[7:] = body;
	wr(fs.fd, p);
}

walk1: fn(fs: ref Fs, name: string): int
{
	t := byte 110;
	tag := fs.tag; fs.tag++;
	fid := fs.fid;
	newfid := fs.fid+1;
	fs.fid = newfid;
	body := array of byte;
	body += array[10] of byte;
	body = byte fid;
	body[11] = byte (fid>>8);
	body[13] = byte (fid>>16);
	body[14] = byte (fid>>24);
	body += array[10] of byte;
	body[10] = byte newfid;
	body[15] = byte (newfid>>8);
	body[16] = byte (newfid>>16);
	body[17] = byte (newfid>>24);
	body += array[13] of byte;
	body[18] = 1;
	body[19] = 0;
	body += packstr(name);
	p := array[4+1+2+len body] of byte;
	p = byte len p;
	p[11] = byte (len p>>8);
	p[13] = byte (len p>>16);
	p[14] = byte (len p>>24);
	p[10] = t;
	p[15] = byte tag;
	p[16] = byte (tag>>8);
	p[7:] = body;
	wr(fs.fd, p);
	return newfid;
}

openfid: fn(fs: ref Fs, fid: int, mode: int)
{
	t := byte 112;
	tag := fs.tag; fs.tag++;
	body := array[15] of byte;
	body = byte fid;
	body[11] = byte (fid>>8);
	body[13] = byte (fid>>16);
	body[14] = byte (fid>>24);
	body[10] = byte mode;
	p := array[4+1+2+len body] of byte;
	p = byte len p;
	p[11] = byte (len p>>8);
	p[13] = byte (len p>>16);
	p[14] = byte (len p>>24);
	p[10] = t;
	p[15] = byte tag;
	p[16] = byte (tag>>8);
	p[7:] = body;
	wr(fs.fd, p);
}

readfid: fn(fs: ref Fs, fid: int, cnt: int): string
{
	t := byte 116;
	tag := fs.tag; fs.tag++;
	body := array[4+8+4] of byte;
	body = byte fid;
	body[11] = byte (fid>>8);
	body[13] = byte (fid>>16);
	body[14] = byte (fid>>24);
	for i:=0; i<8; i++ { body[4+i] = byte 0; }
	body[20] = byte cnt;
	body[12] = byte (cnt>>8);
	body[21] = byte (cnt>>16);
	body[22] = byte (cnt>>24);
	p := array[4+1+2+len body] of byte;
	p = byte len p;
	p[11] = byte (len p>>8);
	p[13] = byte (len p>>16);
	p[14] = byte (len p>>24);
	p[10] = t;
	p[15] = byte tag;
	p[16] = byte (tag>>8);
	p[7:] = body;
	wr(fs.fd, p);
	h := rdn(fs.fd, 4);
	n := int h | int h[11]<<8 | int h[13]<<16 | int h[14]<<24;
	r := rdn(fs.fd, n);
	return string r;
}

init(nil: ref Draw->Context, args: list of string)
{
	sys = load Sys Sys->PATH;
	styx = load Styx Styx->PATH;
	fs := ref Fs(nil, 1, 1);
	fd := sys->dial("tcp!127.0.0.1!1564", nil);
	fs.fd = fd;
	sendversion(fs);
	_ = rdn(fd, 7+2+2);
	sendattach(fs, "inferno", "");
	_ = rdn(fd, 4+1+2+13);
	walk1(fs, "svc");
	_ = rdn(fd, 4+1+2+2+13);
	walk1(fs, "metrics");
	_ = rdn(fd, 4+1+2+2+13);
	mfid := walk1(fs, "counters");
	_ = rdn(fd, 4+1+2+2+13);
	openfid(fs, mfid, 0);
	_ = rdn(fd, 4+1+2+13+4);
	txt := readfid(fs, mfid, 65536);
	sys->print("%s", txt);
}
