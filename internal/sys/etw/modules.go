package etw

import (
	"encoding/binary"
	"sort"
	"strings"
)

// Module is a loaded kernel module (a driver or the kernel itself).
type Module struct {
	Base uint64
	Size uint32
	Name string // file name without the folder: "ndis.sys"
}

// Layout of RTL_PROCESS_MODULES on 64-bit Windows, what
// NtQuerySystemInformation(SystemModuleInformation) returns: a count padded to
// 8 bytes, then 296-byte RTL_PROCESS_MODULE_INFORMATION entries.
const (
	modulesHeader     = 8
	moduleEntrySize   = 296
	moduleOffBase     = 16 // ImageBase (after Section and MappedBase)
	moduleOffSize     = 24 // ImageSize
	moduleOffFileName = 38 // OffsetToFileName
	moduleOffPath     = 40 // FullPathName[256], ANSI
	modulePathLen     = 256
)

// ParseModules decodes a SystemModuleInformation buffer. Entries that do not
// fit in the buffer, or that carry no base address, are skipped.
func ParseModules(b []byte) []Module {
	if len(b) < modulesHeader {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	var out []Module
	for i := 0; i < n; i++ {
		off := modulesHeader + i*moduleEntrySize
		if off+moduleEntrySize > len(b) {
			break
		}
		e := b[off : off+moduleEntrySize]
		base := binary.LittleEndian.Uint64(e[moduleOffBase:])
		size := binary.LittleEndian.Uint32(e[moduleOffSize:])
		if base == 0 || size == 0 {
			continue
		}
		path := e[moduleOffPath : moduleOffPath+modulePathLen]
		if z := indexByte(path, 0); z >= 0 {
			path = path[:z]
		}
		name := string(path)
		if fn := int(binary.LittleEndian.Uint16(e[moduleOffFileName:])); fn > 0 && fn < len(path) {
			name = string(path[fn:])
		} else if k := strings.LastIndexAny(name, `\/`); k >= 0 {
			name = name[k+1:]
		}
		out = append(out, Module{Base: base, Size: size, Name: name})
	}
	return out
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// ModuleMap finds the module that contains an address.
type ModuleMap struct{ mods []Module }

// NewModuleMap sorts the modules by base address; a module listed twice (the
// list is read at the start and at the end of a capture) is kept once.
func NewModuleMap(lists ...[]Module) *ModuleMap {
	seen := map[uint64]bool{}
	var all []Module
	for _, l := range lists {
		for _, m := range l {
			if !seen[m.Base] {
				seen[m.Base] = true
				all = append(all, m)
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Base < all[j].Base })
	return &ModuleMap{mods: all}
}

// Len is how many modules the map holds.
func (m *ModuleMap) Len() int { return len(m.mods) }

// Lookup returns the name of the module whose image contains addr, or "".
func (m *ModuleMap) Lookup(addr uint64) string {
	i := sort.Search(len(m.mods), func(i int) bool { return m.mods[i].Base > addr })
	if i == 0 {
		return ""
	}
	c := m.mods[i-1]
	if addr < c.Base+uint64(c.Size) {
		return c.Name
	}
	return ""
}
