// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aclements/go-obj/arch"
)

type elfTest struct {
	path string
	arch *arch.Arch

	sections      []Section
	textData      []byte
	debugInfoData []byte

	relocs map[string][]relocTest

	nSyms int
	syms  map[int]Sym

	openOnce sync.Once
	openErr  error
	f        File
}

func (test *elfTest) open() (File, error) {
	test.openOnce.Do(func() {
		fp, err := os.Open(filepath.Join("testdata", test.path))
		if err != nil {
			test.openErr = fmt.Errorf("error opening test file: %v", err)
			return
		}

		test.f, err = Open(fp)
		if err != nil {
			test.openErr = fmt.Errorf("Open failed unexpectedly: %v", err)
			return
		}
	})
	if test.openErr != nil {
		return nil, test.openErr
	}
	return test.f, nil
}

func (test *elfTest) openOrSkip(t *testing.T) File {
	f, err := test.open()
	if err != nil {
		t.Skip(err)
	}
	return f
}

func forEachElfTest(t *testing.T, cb func(t *testing.T, test *elfTest)) {
	for _, test := range elfTests {
		t.Run(test.path, func(t *testing.T) {
			cb(t, test)
		})
	}
}

func TestElfOpen(t *testing.T) {
	forEachElfTest(t, func(t *testing.T, test *elfTest) {
		t.Parallel()
		f, err := test.open()
		if err != nil {
			t.Fatal(err)
		}

		// Check the file info.
		info := f.Info()
		if info.Arch != test.arch {
			t.Errorf("want architecture %s, got %s", test.arch, info.Arch)
		}
	})
}

func TestElfClose(t *testing.T) {
	t.Parallel()
	// Since we reuse the files opened for other tests and never close them,
	// this tests file closing separately.
	fp, err := os.Open(filepath.Join("testdata", elfTests[0].path))
	if err != nil {
		t.Fatalf("error opening test file: %v", err)
	}

	f, err := Open(fp)
	if err != nil {
		t.Fatalf("Open failed unexpectedly: %v", err)
	}
	f.Close()
}

func TestElfOpenCorrupted(t *testing.T) {
	t.Parallel()
	// Test that a corrupted ELF file is still detected as ELF, rather than
	// being rejected as an unknown format.
	ident := [16]byte{'\x7f', 'E', 'L', 'F', 42}
	f := bytes.NewReader(ident[:])
	_, err := Open(f)
	if err == nil {
		t.Fatalf("Open succeeded unexpectedly")
	}
	want := "unknown ELF class"
	if !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("want error starting with %q, got %q", want, err.Error())
	}
}

type relocTest struct {
	Addr   uint64
	Type   string
	Symbol string
	Addend int64
}

var relocTests = map[string]map[string][]relocTest{
	// The relocations in the relocatable objects all have target sections.
	"hello-gcc10.3.0-I386-rel.o": {
		".text": {
			{0x00000014, "R_386_PC32", "__x86.get_pc_thunk.ax", -4},
			{0x00000019, "R_386_GOTPC", "_GLOBAL_OFFSET_TABLE_", 1},
			{0x00000022, "R_386_GOTOFF", ".rodata", 0},
			{0x0000002a, "R_386_PLT32", "puts", -4},
		},
	},
	"hello-gcc10.3.0-AMD64-rel.o": {
		".text": {
			{0x000000000016, "R_X86_64_PC32", ".rodata", -4},
			{0x00000000001b, "R_X86_64_PLT32", "puts", -4},
		},
	},
	"hello-gcc10.3.0-I386-dyn": {
		".got": {
			// This relocation is global.
			{0x0804bffc, "R_386_GLOB_DAT", "__gmon_start__", 0},
		},
		// .got.plt is intersting because it has both a global relocation
		// section (which has no relocations that apply) and a targetted
		// relocation section.
		".got.plt": {
			{0x0804c00c, "R_386_JMP_SLOT", "puts", 0x08049040},
			{0x0804c010, "R_386_JMP_SLOT", "__libc_start_main", 0x08049050},
		},
	},
	"hello-gcc10.3.0-AMD64-dyn": {
		".got": {
			// These relocations are global.
			{0x000000403ff0, "R_X86_64_GLOB_DAT", "__libc_start_main", 0},
			{0x000000403ff8, "R_X86_64_GLOB_DAT", "__gmon_start__", 0},
		},
		".got.plt": {
			{0x000000404018, "R_X86_64_JMP_SLOT", "puts", 0},
		},
	},
}

func init() {
	// Attach relocTests (which are hand-written) to elfTests (which are generated).
	for _, elfTest := range elfTests {
		elfTest.relocs = relocTests[elfTest.path]
		delete(relocTests, elfTest.path)
	}
	for path := range relocTests {
		log.Fatalf("relocTest[%q] does not match any elfTest", path)
	}
}

func TestElfSections(t *testing.T) {
	forEachElfTest(t, func(t *testing.T, test *elfTest) {
		t.Parallel()

		f := test.openOrSkip(t)

		sections := f.Sections()

		// Check the sections.
		sectionNames := make(map[string]*Section)
		if len(sections) != len(test.sections) {
			t.Errorf("want %d sections, got %d", len(test.sections), len(sections))
		} else {
			for i, sect := range sections {
				sectionNames[sect.Name] = sect

				// Check basic invariants.
				if sect.ID != SectionID(i) {
					t.Errorf("section %d: want ID %v, got %v", i, i, sect.ID)
				}
				if sect.File != f {
					t.Errorf("section %d: wrong File", i)
				}
				if addr, size := sect.Bounds(); addr != sect.Addr || size != sect.Size {
					t.Errorf("section %d: want bounds %#x/%#x, got %#x/%#x", i, sect.Addr, sect.Size, addr, size)
				}

				// Check that lookup by ID works.
				if got := f.Section(SectionID(i)); sect != got {
					t.Errorf("lookup section %d: want %+v, got %+v", i, sect, got)
				}

				// Check against the expected section data.
				want := test.sections[i]
				if sect.Name != want.Name {
					t.Errorf("section #%d: want name %v, got %v", i, want.Name, sect.Name)
				}
				if sect.ID != want.ID {
					t.Errorf("section %s: want ID %v, got %v", sect.Name, want.ID, sect.ID)
				}
				if sect.RawID != want.RawID {
					t.Errorf("section %s: want raw ID %v, got %v", sect.Name, want.RawID, sect.RawID)
				}
				if sect.Addr != want.Addr || sect.Size != want.Size {
					t.Errorf("section %s: want address/size %#x/%#x, got %#x/%#x", sect.Name, want.Addr, want.Size, sect.Addr, sect.Size)
				}
				if sect.SectionFlags != want.SectionFlags {
					t.Errorf("section %s: want flags %v, got %v", sect.Name, want.SectionFlags, sect.SectionFlags)
				}
			}
		}

		// Check loading .text data.
		if test.textData != nil {
			text := sectionNames[".text"]
			data, err := text.Data(text.Bounds())
			if err != nil {
				t.Errorf("section .text: error getting data: %v", err)
			} else if !bytes.Equal(data.P, test.textData) {
				t.Errorf("section .text: data not as expected")
			}
			// Check loading a sub-slice of the data.
			data, err = text.Data(text.Addr+1, 8)
			if err != nil {
				t.Errorf("section .text: error getting data: %v", err)
			} else if !bytes.Equal(data.P, test.textData[1:][:8]) {
				t.Errorf("section .text: sliced data not as expected")
			}
		}

		// Check loading a compressed section.
		if test.debugInfoData != nil {
			info := sectionNames[".debug_info"]
			data, err := info.Data(info.Bounds())
			if err != nil {
				t.Errorf("section .debug_info: error getting data: %v", err)
			} else if !bytes.Equal(data.P, test.debugInfoData) {
				t.Errorf("section .debug_info: data not as expected")
			}
		}

		// Check loading a NOBITS section.
		bss := sectionNames[".bss"]
		data, err := bss.Data(bss.Bounds())
		if err != nil {
			t.Errorf("section .bss: error getting data: %v", err)
		} else if uint64(len(data.P)) != bss.Size {
			t.Errorf("section .bss: want %d bytes, got %d", bss.Size, len(data.P))
		} else {
			for i, b := range data.P {
				if b != 0 {
					t.Errorf("section .bss: byte %d is not 0", i)
					break
				}
			}
		}

		// Check relocations.
		for sectionName, relocs := range test.relocs {
			sect := sectionNames[sectionName]
			data, err := sect.Data(sect.Bounds())
			if err != nil {
				t.Errorf("section %s: error getting data: %v", sect.Name, err)
			} else if len(data.R) != len(relocs) {
				t.Errorf("section %s: want %d relocations, got %d", sect.Name, len(relocs), len(data.R))
			} else {
				for i, want := range relocs {
					got := relocTest{
						Addr:   data.R[i].Addr,
						Type:   data.R[i].Type.String(),
						Symbol: f.Sym(data.R[i].Symbol).Name,
						Addend: data.R[i].Addend,
					}
					if !reflect.DeepEqual(want, got) {
						t.Errorf("section %s relocation %d:\nwant %+v\ngot %+v", sect.Name, i, want, got)
					}
				}
			}
		}
	})
}

// Test that we mmap or heap-allocate sections as expected.
func TestElfMmap(t *testing.T) {
	// Not parallel because we use a global test hook.

	var mmapCount uint32
	var heapCount uint32
	testMmapSection = func(mmaped bool) {
		if mmaped {
			atomic.AddUint32(&mmapCount, 1)
		} else {
			atomic.AddUint32(&heapCount, 1)
		}
	}
	defer func() { testMmapSection = nil }()

	// Open a fresh file so we're not getting cached sections.
	fp, err := os.Open(filepath.Join("testdata", "hello-gcc10.3.0-AMD64-rel-gz.o"))
	if err != nil {
		t.Fatalf("error opening test file: %v", err)
	}

	f, err := Open(fp)
	if err != nil {
		t.Fatalf("Open failed unexpectedly: %v", err)
	}
	defer f.Close()

	// Load the sections.
	for _, section := range f.Sections() {
		_, err := section.Data(section.Bounds())
		if err != nil {
			t.Fatalf("error reading section: %v", err)
		}
	}

	// Check the counts.
	const wantMmaped = 15
	const wantHeap = 2 + 3 + 1 // 2 zero-length, 1 zero-length NOBITS, 3 compressed sections
	if mmapCount != wantMmaped || heapCount != wantHeap {
		t.Errorf("want %d mmaped + %d heap, got %d+%d", wantMmaped, wantHeap, mmapCount, heapCount)
	}
}
