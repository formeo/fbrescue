package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRewritePage(t *testing.T) {
	page := make([]byte, 1024)
	page[0] = dataPage
	binary.LittleEndian.PutUint16(page[0x16:], 2)
	binary.LittleEndian.PutUint16(page[0x18:], 100)
	binary.LittleEndian.PutUint16(page[0x1a:], 17)
	binary.LittleEndian.PutUint16(page[0x1c:], 200)
	binary.LittleEndian.PutUint16(page[0x1e:], 24)

	normal := page[100:117]
	normal[12] = 7
	copy(normal[13:], []byte{1, 2, 3, 4})
	fragment := page[200:224]
	binary.LittleEndian.PutUint16(fragment[10:], incompleteFlag)
	binary.LittleEndian.PutUint32(fragment[16:], 55)
	binary.LittleEndian.PutUint16(fragment[20:], 9)
	copy(fragment[22:], []byte{5, 6})

	if err := rewritePage(page); err != nil {
		t.Fatal(err)
	}
	off := binary.LittleEndian.Uint16(page[0x18:])
	n := binary.LittleEndian.Uint16(page[0x1a:])
	if n != 20 {
		t.Fatalf("ordinary record length = %d, want 20", n)
	}
	normal = page[off : off+n]
	if flags := binary.LittleEndian.Uint16(normal[10:]); flags != longTxFlag {
		t.Errorf("flags = %#x, want long transaction", flags)
	}
	if high := binary.LittleEndian.Uint16(normal[14:]); high != 1 {
		t.Errorf("high word = %d, want 1", high)
	}
	if !bytes.Equal(normal[16:], []byte{1, 2, 3, 4}) {
		t.Errorf("ordinary record data changed: %v", normal[16:])
	}

	off = binary.LittleEndian.Uint16(page[0x1c:])
	n = binary.LittleEndian.Uint16(page[0x1e:])
	if n != 24 {
		t.Fatalf("fragment length = %d, want unchanged 24", n)
	}
	fragment = page[off : off+n]
	if high := binary.LittleEndian.Uint16(fragment[14:]); high != 1 {
		t.Errorf("fragment high word = %d, want 1", high)
	}
	if next := binary.LittleEndian.Uint32(fragment[16:]); next != 55 {
		t.Errorf("next fragment page = %d, want 55", next)
	}
	if !bytes.Equal(fragment[22:], []byte{5, 6}) {
		t.Errorf("fragment data changed: %v", fragment[22:])
	}
}
