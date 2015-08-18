package ftp

import (
	"testing"
	"time"
)

var thisYear, _, _ = time.Now().Date()

type line struct {
	line      string
	name      string
	size      uint64
	entryType EntryType
	time      time.Time
}

var listTests = []line{
	// UNIX ls -l style
	{"drwxr-xr-x    3 110      1002            3 Dec 02  2009 pub", "pub", 0, EntryTypeFolder, time.Date(2009, time.December, 2, 0, 0, 0, 0, time.UTC)},
	{"drwxr-xr-x    3 110      1002            3 Dec 02  2009 p u b", "p u b", 0, EntryTypeFolder, time.Date(2009, time.December, 2, 0, 0, 0, 0, time.UTC)},
	{"-rwxr-xr-x    3 110      1002            1234567 Dec 02  2009 fileName", "fileName", 1234567, EntryTypeFile, time.Date(2009, time.December, 2, 0, 0, 0, 0, time.UTC)},
	{"lrwxrwxrwx   1 root     other          7 Jan 25 00:17 bin -> usr/bin", "bin -> usr/bin", 0, EntryTypeLink, time.Date(thisYear, time.January, 25, 0, 17, 0, 0, time.UTC)},

	// Another ls style
	{"drwxr-xr-x               folder        0 Aug 15 05:49 !!!-Tipp des Haus!", "!!!-Tipp des Haus!", 0, EntryTypeFolder, time.Date(thisYear, time.August, 15, 5, 49, 0, 0, time.UTC)},
	{"drwxrwxrwx               folder        0 Aug 11 20:32 P0RN", "P0RN", 0, EntryTypeFolder, time.Date(thisYear, time.August, 11, 20, 32, 0, 0, time.UTC)},
	{"-rw-r--r--        0   18446744073709551615 18446744073709551615 Nov 16  2006 VIDEO_TS.VOB", "VIDEO_TS.VOB", 18446744073709551615, EntryTypeFile, time.Date(2006, time.November, 16, 0, 0, 0, 0, time.UTC)},

	// Microsoft's FTP servers for Windows
	{"----------   1 owner    group         1803128 Jul 10 10:18 ls-lR.Z", "ls-lR.Z", 1803128, EntryTypeFile, time.Date(thisYear, time.July, 10, 10, 18, 0, 0, time.UTC)},
	{"d---------   1 owner    group               0 May  9 19:45 Softlib", "Softlib", 0, EntryTypeFolder, time.Date(thisYear, time.May, 9, 19, 45, 0, 0, time.UTC)},

	// WFTPD for MSDOS
	{"-rwxrwxrwx   1 noone    nogroup      322 Aug 19  1996 message.ftp", "message.ftp", 322, EntryTypeFile, time.Date(1996, time.August, 19, 0, 0, 0, 0, time.UTC)},

	// RFC3659 format: https://tools.ietf.org/html/rfc3659#section-7
	{"modify=20150813224845;perm=fle;type=cdir;unique=119FBB87U4;UNIX.group=0;UNIX.mode=0755;UNIX.owner=0; .", ".", 0, EntryTypeFolder, time.Date(2015, time.August, 13, 22, 48, 45, 0, time.UTC)},
	{"modify=20150813224845;perm=fle;type=pdir;unique=119FBB87U4;UNIX.group=0;UNIX.mode=0755;UNIX.owner=0; ..", "..", 0, EntryTypeFolder, time.Date(2015, time.August, 13, 22, 48, 45, 0, time.UTC)},
	{"modify=20150806235817;perm=fle;type=dir;unique=1B20F360U4;UNIX.group=0;UNIX.mode=0755;UNIX.owner=0; movies", "movies", 0, EntryTypeFolder, time.Date(2015, time.August, 6, 23, 58, 17, 0, time.UTC)},
	{"modify=20150814172949;perm=flcdmpe;type=dir;unique=85A0C168U4;UNIX.group=0;UNIX.mode=0777;UNIX.owner=0; _upload", "_upload", 0, EntryTypeFolder, time.Date(2015, time.August, 14, 17, 29, 49, 0, time.UTC)},
	{"modify=20150813175250;perm=adfr;size=951;type=file;unique=119FBB87UE;UNIX.group=0;UNIX.mode=0644;UNIX.owner=0; welcome.msg", "welcome.msg", 951, EntryTypeFile, time.Date(2015, time.August, 13, 17, 52, 50, 0, time.UTC)},
}

// Not supported, at least we should properly return failure
var listTestsFail = []line{
	{"d [R----F--] supervisor            512       Jan 16 18:53 login", "login", 0, EntryTypeFolder, time.Date(thisYear, time.January, 16, 18, 53, 0, 0, time.UTC)},
	{"- [R----F--] rhesus             214059       Oct 20 15:27 cx.exe", "cx.exe", 0, EntryTypeFile, time.Date(thisYear, time.October, 20, 15, 27, 0, 0, time.UTC)},
}

func TestParseListLine(t *testing.T) {
	for _, lt := range listTests {
		entry, err := parseListLine(lt.line)
		if err != nil {
			t.Errorf("parseListLine(%v) returned err = %v", lt.line, err)
			continue
		}
		if entry.Name != lt.name {
			t.Errorf("parseListLine(%v).Name = '%v', want '%v'", lt.line, entry.Name, lt.name)
		}
		if entry.Type != lt.entryType {
			t.Errorf("parseListLine(%v).EntryType = %v, want %v", lt.line, entry.Type, lt.entryType)
		}
		if entry.Size != lt.size {
			t.Errorf("parseListLine(%v).Size = %v, want %v", lt.line, entry.Size, lt.size)
		}
		if entry.Time.Unix() != lt.time.Unix() {
			t.Errorf("parseListLine(%v).Time = %v, want %v", lt.line, entry.Time, lt.time)
		}
	}
	for _, lt := range listTestsFail {
		_, err := parseListLine(lt.line)
		if err == nil {
			t.Errorf("parseListLine(%v) expected to fail", lt.line)
		}
	}
}
