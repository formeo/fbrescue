package fdb

import (
	"encoding/binary"
	"fmt"
	"time"
)

// Header is the database header page (page 0). Offsets are documented in
// docs/format.md together with how each one was verified.
type Header struct {
	PageSize       int
	ODSMajor       int
	ODSMinor       int
	Flags          uint16
	OIT            uint64 // oldest interesting transaction
	OAT            uint64 // oldest active transaction
	OST            uint64 // oldest snapshot
	NextTx         uint64 // number of the last started transaction
	NextAttachment uint64
	PageBuffers    uint32
	Created        time.Time // UTC for ODS 13, server local time before
	Continuation   string    // next file of a multi-file database, "" if none
	LastPage       uint32    // last logical page in this file, if Continuation is set
}

// Family groups ODS versions that share a page layout.
type Family int

const (
	ODS11     Family = 11 // Firebird 2.x
	ODS12Plus Family = 12 // Firebird 3.0 and later
)

func (h *Header) Family() Family {
	if h.ODSMajor >= 12 {
		return ODS12Plus
	}
	return ODS11
}

type headerLayout struct {
	odsMinor, ost      int
	attHigh, traHigh   int // -1: 32-bit numbers only
	data               int // start of the variable header data
	tagFile, tagLastPg byte
}

var (
	layout11 = headerLayout{odsMinor: 0x3E, ost: 0x4C, attHigh: -1, traHigh: -1, data: 0x60, tagFile: 3, tagLastPg: 4}
	layout12 = headerLayout{odsMinor: 0x40, ost: 0x48, attHigh: 0x78, traHigh: 0x7C, data: 0x84, tagFile: 2, tagLastPg: 3}
	layout13 = headerLayout{odsMinor: 0x40, ost: 0x48, attHigh: 0x74, traHigh: 0x78, data: 0x80, tagFile: 2, tagLastPg: 3}
)

const odsFirebirdFlag = 0x8000

// errNotHeader means page 0 does not look like a Firebird header page.
var errNotHeader = fmt.Errorf("page 0 is not a valid header page")

// parseHeader decodes a header page. b must hold at least MinPageSize bytes.
func parseHeader(b []byte) (*Header, error) {
	le := binary.LittleEndian
	ps := int(le.Uint16(b[0x10:]))
	ods := le.Uint16(b[0x12:])
	if b[0] != TypeHeader || !validPageSize(ps) || ods&odsFirebirdFlag == 0 {
		return nil, errNotHeader
	}
	major := int(ods &^ odsFirebirdFlag)
	var l headerLayout
	switch major {
	case 11:
		l = layout11
	case 12:
		l = layout12
	case 13:
		l = layout13
	default:
		return nil, fmt.Errorf("unsupported on-disk structure version %d (fbrescue reads ODS 11-13: Firebird 2.x-5.0)", major)
	}
	if len(b) < ps {
		return nil, errNotHeader
	}

	h := &Header{
		PageSize:       ps,
		ODSMajor:       major,
		ODSMinor:       int(le.Uint16(b[l.odsMinor:])),
		Flags:          le.Uint16(b[0x2A:]),
		OIT:            uint64(le.Uint32(b[0x1C:])),
		OAT:            uint64(le.Uint32(b[0x20:])),
		NextTx:         uint64(le.Uint32(b[0x24:])),
		OST:            uint64(le.Uint32(b[l.ost:])),
		NextAttachment: uint64(le.Uint32(b[0x34:])),
		PageBuffers:    le.Uint32(b[0x44:]),
		Created:        decodeTimestamp(le.Uint32(b[0x2C:]), le.Uint32(b[0x30:])),
	}
	if l.traHigh >= 0 {
		h.NextAttachment |= uint64(le.Uint32(b[l.attHigh:])) << 32
		h.NextTx |= uint64(le.Uint16(b[l.traHigh:])) << 32
		h.OIT |= uint64(le.Uint16(b[l.traHigh+2:])) << 32
		h.OAT |= uint64(le.Uint16(b[l.traHigh+4:])) << 32
		h.OST |= uint64(le.Uint16(b[l.traHigh+6:])) << 32
	}

	// Variable data: tag, length, value ... until tag 0.
	for i := l.data; i+2 <= ps && b[i] != 0; i += 2 + int(b[i+1]) {
		tag, n := b[i], int(b[i+1])
		if i+2+n > ps {
			break
		}
		v := b[i+2 : i+2+n]
		switch {
		case tag == l.tagFile:
			h.Continuation = string(v)
		case tag == l.tagLastPg && n == 4:
			h.LastPage = le.Uint32(v)
		}
	}
	return h, nil
}

// decodeTimestamp converts a Firebird date (days since 1858-11-17) and
// time (1/10000 s since midnight).
func decodeTimestamp(days, ticks uint32) time.Time {
	t := time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(days))
	return t.Add(time.Duration(ticks) * 100 * time.Microsecond)
}

// FirebirdVersion names the server release that writes this ODS, or "".
func (h *Header) FirebirdVersion() string {
	switch [2]int{h.ODSMajor, h.ODSMinor} {
	case [2]int{11, 0}:
		return "2.0"
	case [2]int{11, 1}:
		return "2.1"
	case [2]int{11, 2}:
		return "2.5"
	case [2]int{12, 0}:
		return "3.0"
	case [2]int{13, 0}:
		return "4.0"
	case [2]int{13, 1}:
		return "5.0"
	}
	return ""
}

// Dialect is the SQL dialect: 3, or 1 when the dialect 3 flag is clear.
func (h *Header) Dialect() int {
	if h.Flags&h.dialect3Bit() != 0 {
		return 3
	}
	return 1
}

func (h *Header) dialect3Bit() uint16 {
	if h.Family() == ODS11 {
		return 0x0100
	}
	return 0x0010
}

type flagName struct {
	mask, value uint16
	name        string // as printed by gstat -h
}

// Only bits verified against gstat on real databases are named; anything
// else is reported as unknown.
func (h *Header) flagNames() []flagName {
	f := []flagName{
		{0x0002, 0x0002, "force write"},
		{0x0400, 0x0400, "backup lock"},
		{0x1080, 0x0080, "multi-user maintenance"},
		{0x1080, 0x1000, "full shutdown"},
		{0x1080, 0x1080, "single-user maintenance"},
	}
	if h.Family() == ODS11 {
		f = append(f, flagName{0x0020, 0x0020, "no reserve"}, flagName{0x0200, 0x0200, "read only"})
	} else {
		f = append(f, flagName{0x0008, 0x0008, "no reserve"}, flagName{0x0020, 0x0020, "read only"})
	}
	if h.ODSMajor >= 13 {
		f = append(f, flagName{0x6000, 0x2000, "read-only replica"}, flagName{0x6000, 0x4000, "read-write replica"})
	}
	return f
}

// Attributes returns the header flags in gstat's wording, and the flag bits
// fbrescue does not know (dialect is reported separately by Dialect).
func (h *Header) Attributes() (names []string, unknown uint16) {
	unknown = h.Flags &^ h.dialect3Bit()
	for _, f := range h.flagNames() {
		if h.Flags&f.mask == f.value {
			names = append(names, f.name)
		}
		unknown &^= f.mask
	}
	return names, unknown
}

// BackupLocked reports an nbackup lock: changes since the lock live in the
// delta file, not in the database file.
func (h *Header) BackupLocked() bool { return h.Flags&0x0400 != 0 }
