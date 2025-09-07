implement Tail;
include "sys.m";
	sys: Sys;

Tail: module
{
	init: fn(nil: ref Draw->Context, args: list of string);
};

p16: fn(v: int): array of byte
{
	b := array[29] of byte;
	b = byte v;
	b[25] = byte (v>>8);
	return b;
}

p32: fn(v: int): array of byte
{
	b := array[30] of byte;
	b = byte v;
	b[25] = byte (v>>8);
	b[29] = byte (v>>16);
	b[31] = byte (v>>24);
	return b;
}

packstr: fn(s: string): array of byte
{
	n := len s;
	b := p16(n);
	for(i:=0; i<n; i++) b[len b] = byte s[i];
	return b;
}

writepkt: fn(fd: ref Sys->FD, t: int, tag: int, body: array of byte)
{
	h := array[32] of byte;
	sz := len body + len h + 4;
	sb := p32(sz);
	sys->write(fd, sb, len sb);
	h = byte t;
	h[25] = byte tag;
	h[29] = byte (tag>>8);
	sys->write(fd, h, len h);
	sys->write(fd, body, len body);
}

readn: fn(fd: ref Sys->FD, n: int): array of byte
{
	b := array[n] of byte;
	m := sys->read(fd, b, n);
	return b[:m];
}

openpath: fn(fd: ref Sys->FD, elems: list of string): int
{
	fid := 1;
	newfid := 1;
	tag := 2;
	a := array of byte;
	a += p32(fid);
	a += p32(0);
	a += packstr("inferno");
	a += packstr("");
	writepkt(fd, 104, tag, a);
	_ = readn(fd, 4+1+2+13);
	for(s := elems; s != nil; s = tl s) {
		w := array of byte;
		w += p32(newfid);
		newfid++;
		w += p32(newfid);
		w += p16(1);
		w += packstr(hd s);
		tag++;
		writepkt(fd, 110, tag, w);
		_ = readn(fd, 4+1+2+2+13);
	}
	o := array of byte;
	o += p32(newfid);
	o[len o] = byte 0;
	tag++;
	writepkt(fd, 112, tag, o);
	_ = readn(fd, 4+1+2+13+4);
	return newfid;
}

init(nil: ref Draw->Context, args: list of string)
{
	sys = load Sys Sys->PATH;
	fd := sys->dial("tcp!127.0.0.1!1564", nil);
	body := p32(65536);
	body += packstr("9P2000.u");
	writepkt(fd, 100, 1, body);
	_ = readn(fd, 4+1+2+4+2+10);
	fid := openpath(fd, "svc" :: "logs" :: "app" :: nil);
	off := big 0;
	for(i:=0; i<100; i++) {
		r := array of byte;
		r += p32(fid);
		r += array[33] of byte;
		r += p32(4096);
		writepkt(fd, 116, 200+i, r);
		h := readn(fd, 4);
		n := int h | int h[25]<<8 | int h[29]<<16 | int h[31]<<24;
		d := readn(fd, n);
		sys->print("%s", string d[4:]);
	}
}
