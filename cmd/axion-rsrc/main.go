// Command axion-rsrc builds the Windows resources of Axion.exe without
// llvm-windres: the icon (build\axion.ico, generated from the official mark),
// the application manifest and the version information, written as a COFF
// object (cmd\axion\rsrc_windows_amd64.syso) that the Go linker embeds.
//
//	go run ./cmd/axion-rsrc -version 0.9.1
//	go run ./cmd/axion-rsrc -ico-only            # only regenerates build\axion.ico
//
// Only the standard library is used. The layout of the object is the one the
// Go linker expects from windres and rsrc: one .rsrc section with the resource
// directory tree and one ADDR32NB relocation per data entry.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	version := flag.String("version", "", "versão maior.menor.correção (obrigatória, salvo com -ico-only)")
	mark := flag.String("mark", "build/axion-mark.png", "PNG quadrado da marca oficial")
	icoOnly := flag.Bool("ico-only", false, "só gera o build/axion.ico")
	out := flag.String("out", "cmd/axion/rsrc_windows_amd64.syso", "objeto de recursos")
	flag.Parse()

	icoPath := filepath.Join(*root, "build", "axion.ico")
	ico, err := buildICO(filepath.Join(*root, *mark))
	if err != nil {
		fail("ícone: %v", err)
	}
	if err := os.WriteFile(icoPath, ico.file, 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Printf("ícone: %s (%d tamanhos, %d bytes)\n", icoPath, len(ico.images), len(ico.file))
	if *icoOnly {
		return
	}

	v, err := parseVersion(*version)
	if err != nil {
		fail("%v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(*root, "build", "axion.manifest"))
	if err != nil {
		fail("%v", err)
	}
	manifest = setManifestVersion(manifest, *version)
	if err := os.WriteFile(filepath.Join(*root, "build", "axion.manifest"), manifest, 0o644); err != nil {
		fail("%v", err)
	}
	obj := buildCOFF(ico, manifest, versionInfo(v, *version))
	outPath := filepath.Join(*root, *out)
	if err := os.WriteFile(outPath, obj, 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Printf("recursos: %s (versão %s, %d bytes)\n", outPath, *version, len(obj))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "axion-rsrc: "+format+"\n", a...)
	os.Exit(1)
}

func parseVersion(s string) ([3]uint16, error) {
	var v [3]uint16
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("versão inválida %q (use maior.menor.correção)", s)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return v, fmt.Errorf("versão inválida %q: %v", s, err)
		}
		v[i] = uint16(n)
	}
	return v, nil
}

var reManifestVersion = regexp.MustCompile(`version="[0-9.]+" processorArchitecture="amd64"`)

func setManifestVersion(m []byte, v string) []byte {
	return reManifestVersion.ReplaceAll(m, []byte(`version="`+v+`.0" processorArchitecture="amd64"`))
}

// ---------------------------------------------------------------------------
// Icon: the square mark with rounded corners, in the sizes Windows uses.
// ---------------------------------------------------------------------------

type icoImage struct {
	size int
	data []byte // DIB (BITMAPINFOHEADER + pixels + mask) or PNG
}

type icoFile struct {
	images []icoImage
	file   []byte
}

var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

func buildICO(markPath string) (*icoFile, error) {
	f, err := os.Open(markPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", markPath, err)
	}
	b := src.Bounds()
	if b.Dx() != b.Dy() || b.Dx() < 256 {
		return nil, fmt.Errorf("%s: a marca precisa ser quadrada e ter pelo menos 256 px (tem %dx%d)", markPath, b.Dx(), b.Dy())
	}
	base := rounded(toNRGBA(src), 0.22)
	ico := &icoFile{}
	for _, s := range icoSizes {
		img := downscale(base, s)
		var data []byte
		if s >= 64 {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				return nil, err
			}
			data = buf.Bytes()
		} else {
			data = dib(img)
		}
		ico.images = append(ico.images, icoImage{size: s, data: data})
	}
	// ICONDIR + ICONDIRENTRY per image, then the images.
	var hdr bytes.Buffer
	w := func(v any) { _ = binary.Write(&hdr, binary.LittleEndian, v) }
	w(uint16(0))
	w(uint16(1))
	w(uint16(len(ico.images)))
	off := 6 + 16*len(ico.images)
	var body bytes.Buffer
	for _, im := range ico.images {
		w(sizeByte(im.size))
		w(sizeByte(im.size))
		w(uint8(0))
		w(uint8(0))
		w(uint16(1))
		w(uint16(32))
		w(uint32(len(im.data)))
		w(uint32(off + body.Len()))
		body.Write(im.data)
	}
	ico.file = append(hdr.Bytes(), body.Bytes()...)
	return ico, nil
}

func sizeByte(s int) uint8 {
	if s >= 256 {
		return 0
	}
	return uint8(s)
}

func toNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst
}

// rounded cuts the corners of a square image with a radius of `frac` of the
// side; the edge is anti-aliased with 4x4 samples per pixel.
func rounded(img *image.NRGBA, frac float64) *image.NRGBA {
	n := img.Bounds().Dx()
	r := frac * float64(n)
	out := image.NewNRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	inside := func(x, y float64) bool {
		// Distance to the nearest corner circle centre, only in the corner squares.
		cx := r
		if x > float64(n)-r {
			cx = float64(n) - r
		} else if x >= r {
			return true
		}
		cy := r
		if y > float64(n)-r {
			cy = float64(n) - r
		} else if y >= r {
			return true
		}
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= r*r
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if float64(x) >= r && float64(x) < float64(n)-r && float64(y) >= r && float64(y) < float64(n)-r {
				continue
			}
			hits := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					if inside(float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4) {
						hits++
					}
				}
			}
			i := out.PixOffset(x, y)
			out.Pix[i+3] = uint8(int(out.Pix[i+3]) * hits / 16)
		}
	}
	return out
}

// downscale averages the source pixels covered by each destination pixel
// (box filter, premultiplied alpha), which is what icon sizes need.
func downscale(src *image.NRGBA, size int) *image.NRGBA {
	n := src.Bounds().Dx()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(n) / float64(size)
	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			x0, x1 := int(float64(dx)*scale), int(float64(dx+1)*scale)
			y0, y1 := int(float64(dy)*scale), int(float64(dy+1)*scale)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			var r, g, b, a, cnt float64
			for y := y0; y < y1 && y < n; y++ {
				for x := x0; x < x1 && x < n; x++ {
					i := src.PixOffset(x, y)
					pa := float64(src.Pix[i+3]) / 255
					r += float64(src.Pix[i]) * pa
					g += float64(src.Pix[i+1]) * pa
					b += float64(src.Pix[i+2]) * pa
					a += pa
					cnt++
				}
			}
			o := dst.PixOffset(dx, dy)
			if a > 0 {
				dst.Pix[o] = uint8(r/a + 0.5)
				dst.Pix[o+1] = uint8(g/a + 0.5)
				dst.Pix[o+2] = uint8(b/a + 0.5)
			}
			dst.Pix[o+3] = uint8(a/cnt*255 + 0.5)
		}
	}
	return dst
}

// dib encodes a 32-bit BMP for an icon entry: BITMAPINFOHEADER with doubled
// height, BGRA rows bottom-up and the 1-bit AND mask.
func dib(img *image.NRGBA) []byte {
	n := img.Bounds().Dx()
	maskRow := ((n + 31) / 32) * 4
	var buf bytes.Buffer
	w := func(v any) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	w(uint32(40))
	w(int32(n))
	w(int32(n * 2))
	w(uint16(1))
	w(uint16(32))
	w(uint32(0))
	w(uint32(n*n*4 + maskRow*n))
	w(int32(0))
	w(int32(0))
	w(uint32(0))
	w(uint32(0))
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			i := img.PixOffset(x, y)
			buf.Write([]byte{img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]})
		}
	}
	for y := n - 1; y >= 0; y-- {
		row := make([]byte, maskRow)
		for x := 0; x < n; x++ {
			if img.Pix[img.PixOffset(x, y)+3] == 0 {
				row[x/8] |= 0x80 >> (x % 8)
			}
		}
		buf.Write(row)
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// Version information (VS_VERSIONINFO).
// ---------------------------------------------------------------------------

const (
	langPTBR = 0x0416
	langENUS = 0x0409
	cpUTF16  = 1200
)

func versionInfo(v [3]uint16, text string) []byte {
	fixed := new(bytes.Buffer)
	w := func(x any) { _ = binary.Write(fixed, binary.LittleEndian, x) }
	w(uint32(0xFEEF04BD))              // dwSignature
	w(uint32(0x00010000))              // dwStrucVersion
	w(uint32(v[0])<<16 | uint32(v[1])) // dwFileVersionMS
	w(uint32(v[2]) << 16)              // dwFileVersionLS
	w(uint32(v[0])<<16 | uint32(v[1])) // dwProductVersionMS
	w(uint32(v[2]) << 16)              // dwProductVersionLS
	w(uint32(0x3f))                    // dwFileFlagsMask
	w(uint32(0))                       // dwFileFlags
	w(uint32(0x40004))                 // dwFileOS VOS_NT_WINDOWS32
	w(uint32(1))                       // dwFileType VFT_APP
	w(uint32(0))                       // dwFileSubtype
	w(uint64(0))                       // dwFileDate
	strs := [][2]string{
		{"CompanyName", "Axion"},
		{"FileDescription", "Axion Optimizer"},
		{"FileVersion", text},
		{"InternalName", "Axion"},
		{"LegalCopyright", "© 2026 Vinicius Hammoud · licença MIT"},
		{"OriginalFilename", "Axion.exe"},
		{"ProductName", "Axion Optimizer"},
		{"ProductVersion", text},
	}
	var table []byte
	for _, kv := range strs {
		table = append(table, vsBlock(kv[0], utf16z(kv[1]), 1, len(utf16z(kv[1]))/2)...)
	}
	stringFileInfo := vsBlock("StringFileInfo", vsBlock(fmt.Sprintf("%04x%04x", langPTBR, cpUTF16), table, 1, 0), 1, 0)
	var tr [4]byte
	binary.LittleEndian.PutUint16(tr[0:], langPTBR)
	binary.LittleEndian.PutUint16(tr[2:], cpUTF16)
	varFileInfo := vsBlock("VarFileInfo", vsBlock("Translation", tr[:], 0, 4), 1, 0)
	return vsBlock("VS_VERSION_INFO", append(fixed.Bytes(), append(stringFileInfo, varFileInfo...)...), 0, fixed.Len())
}

// vsBlock lays out one VS_VERSIONINFO node: wLength, wValueLength, wType,
// szKey (UTF-16, null-terminated), padding to 4, value/children.
func vsBlock(key string, value []byte, typ uint16, valueLen int) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint16(0)) // wLength, patched below
	_ = binary.Write(&b, binary.LittleEndian, uint16(valueLen))
	_ = binary.Write(&b, binary.LittleEndian, typ)
	b.Write(utf16z(key))
	pad4(&b)
	b.Write(value)
	pad4(&b)
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	return out
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2*len(u)+2)
	for _, c := range u {
		out = binary.LittleEndian.AppendUint16(out, c)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// ---------------------------------------------------------------------------
// COFF object with the .rsrc section.
// ---------------------------------------------------------------------------

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
)

type resource struct {
	typ, id, lang uint16
	data          []byte
}

func buildCOFF(ico *icoFile, manifest, version []byte) []byte {
	var res []resource
	var group bytes.Buffer
	gw := func(v any) { _ = binary.Write(&group, binary.LittleEndian, v) }
	gw(uint16(0))
	gw(uint16(1))
	gw(uint16(len(ico.images)))
	for i, im := range ico.images {
		res = append(res, resource{rtIcon, uint16(i + 1), langENUS, im.data})
		gw(sizeByte(im.size))
		gw(sizeByte(im.size))
		gw(uint8(0))
		gw(uint8(0))
		gw(uint16(1))
		gw(uint16(32))
		gw(uint32(len(im.data)))
		gw(uint16(i + 1))
	}
	res = append(res,
		resource{rtGroupIcon, 1, langENUS, group.Bytes()},
		resource{rtVersion, 1, langPTBR, version},
		resource{rtManifest, 1, langENUS, manifest})

	// Directory tree: type -> id -> language -> data entry. Entries of each
	// directory are sorted by id (Windows does a binary search).
	root := &node{}
	for i := range res {
		r := &res[i]
		t := child(&root.kids, r.typ)
		n := child(&t.kids, r.id)
		l := child(&n.kids, r.lang)
		l.res = r
	}

	// Sizes: directories first (breadth-first, like windres), then data
	// entries, then the data itself (aligned to 8).
	var dirs []*node
	queue := []*node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.res != nil {
			continue
		}
		dirs = append(dirs, n)
		queue = append(queue, n.kids...)
	}
	dirOff := map[*node]uint32{}
	off := uint32(0)
	for _, d := range dirs {
		dirOff[d] = off
		off += 16 + 8*uint32(len(d.kids))
	}
	entryOff := map[*resource]uint32{}
	for i := range res {
		entryOff[&res[i]] = off
		off += 16
	}
	dataOff := map[*resource]uint32{}
	for i := range res {
		off = align(off, 8)
		dataOff[&res[i]] = off
		off += uint32(len(res[i].data))
	}
	sectionSize := align(off, 8)

	sec := make([]byte, sectionSize)
	for _, d := range dirs {
		o := dirOff[d]
		binary.LittleEndian.PutUint16(sec[o+14:], uint16(len(d.kids)))
		for i, k := range d.kids {
			e := o + 16 + 8*uint32(i)
			binary.LittleEndian.PutUint32(sec[e:], uint32(k.id))
			if k.res != nil {
				binary.LittleEndian.PutUint32(sec[e+4:], entryOff[k.res])
			} else {
				binary.LittleEndian.PutUint32(sec[e+4:], dirOff[k]|0x80000000)
			}
		}
	}
	var relocs bytes.Buffer
	for i := range res {
		r := &res[i]
		e := entryOff[r]
		binary.LittleEndian.PutUint32(sec[e:], dataOff[r]) // relocated by the linker (ADDR32NB)
		binary.LittleEndian.PutUint32(sec[e+4:], uint32(len(r.data)))
		copy(sec[dataOff[r]:], r.data)
		_ = binary.Write(&relocs, binary.LittleEndian, e)         // VirtualAddress
		_ = binary.Write(&relocs, binary.LittleEndian, uint32(0)) // symbol 0: the section
		_ = binary.Write(&relocs, binary.LittleEndian, uint16(3)) // IMAGE_REL_AMD64_ADDR32NB
	}

	const fileHeader, sectionHeader = 20, 40
	rawOff := uint32(fileHeader + sectionHeader)
	relocOff := rawOff + sectionSize
	symOff := relocOff + uint32(relocs.Len())

	var out bytes.Buffer
	w := func(v any) { _ = binary.Write(&out, binary.LittleEndian, v) }
	// IMAGE_FILE_HEADER
	w(uint16(0x8664)) // amd64
	w(uint16(1))
	w(uint32(0))
	w(symOff)
	w(uint32(1))
	w(uint16(0))
	w(uint16(0x0004)) // IMAGE_FILE_LINE_NUMS_STRIPPED
	// IMAGE_SECTION_HEADER
	var name [8]byte
	copy(name[:], ".rsrc")
	out.Write(name[:])
	w(uint32(0)) // VirtualSize
	w(uint32(0)) // VirtualAddress
	w(sectionSize)
	w(rawOff)
	w(relocOff)
	w(uint32(0))
	w(uint16(len(res)))
	w(uint16(0))
	w(uint32(0x40000040)) // INITIALIZED_DATA | MEM_READ
	out.Write(sec)
	out.Write(relocs.Bytes())
	// Symbol table: the section symbol, then the (empty) string table.
	out.Write(name[:])
	w(uint32(0))
	w(uint16(1))
	w(uint16(0))
	w(uint8(3)) // IMAGE_SYM_CLASS_STATIC
	w(uint8(0))
	w(uint32(4))
	return out.Bytes()
}

func align(v, a uint32) uint32 { return (v + a - 1) / a * a }

// node is one directory (or, with res set, one language leaf) of the tree.
type node struct {
	id   uint16
	kids []*node
	res  *resource
}

// child returns the child with that id, inserting it in id order.
func child(kids *[]*node, id uint16) *node {
	for i, k := range *kids {
		if k.id == id {
			return k
		}
		if k.id > id {
			n := &node{id: id}
			*kids = append((*kids)[:i], append([]*node{n}, (*kids)[i:]...)...)
			return n
		}
	}
	n := &node{id: id}
	*kids = append(*kids, n)
	return n
}
