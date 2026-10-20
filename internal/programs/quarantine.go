package programs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"axion/internal/startup"
	"axion/internal/sys"
)

// QItem is one leftover moved to the quarantine.
type QItem struct {
	Kind     string        `json:"kind"`
	Original string        `json:"original"`
	Value    string        `json:"value,omitempty"`  // run value name
	Stored   string        `json:"stored,omitempty"` // relative path inside the batch (files and folders)
	Reg      *RegTree      `json:"reg,omitempty"`
	RegValue *sys.RegValue `json:"regValue,omitempty"`
	Bytes    int64         `json:"bytes"`
	Restored bool          `json:"restored,omitempty"`
	// Partial: removing the original failed half-way (a locked file or
	// sub-key); what was saved here can still be put back.
	Partial bool `json:"partial,omitempty"`
}

// Batch is one quarantine operation (one program).
type Batch struct {
	ID       string    `json:"id"`
	Program  string    `json:"program"`
	At       time.Time `json:"at"`
	Items    []QItem   `json:"items"`
	Errors   []string  `json:"errors,omitempty"`
	Restored bool      `json:"restored,omitempty"`
	Bytes    int64     `json:"bytes"`
	// MAC authenticates the manifest with the installation's record key;
	// Restore (which runs with administrator rights) only accepts batches
	// with a valid MAC. Verified is computed when listing.
	MAC      string `json:"mac,omitempty"`
	Verified bool   `json:"verified"`
}

func batchMAC(key []byte, b *Batch) string {
	if len(key) == 0 {
		return ""
	}
	c := *b
	c.MAC, c.Verified = "", false
	data, err := json.Marshal(&c)
	if err != nil {
		return ""
	}
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}

func verifyBatch(key []byte, b *Batch) bool {
	want := batchMAC(key, b)
	return want != "" && hmac.Equal([]byte(b.MAC), []byte(want))
}

// Keep is how long a batch stays before it is deleted for good.
const Keep = 30 * 24 * time.Hour

func manifestPath(root, id string) string { return filepath.Join(root, id, "manifest.json") }

func saveBatch(root string, b *Batch, key []byte) error {
	b.Verified = false
	b.MAC = batchMAC(key, b)
	data, err := json.MarshalIndent(b, "", " ")
	if err != nil {
		return err
	}
	tmp := manifestPath(root, b.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, manifestPath(root, b.ID))
}

// Quarantine moves the chosen leftovers into root\<batch>. Folders and files
// are moved (copied and removed when on another drive), registry keys and
// run values are saved in the manifest and removed, tasks are disabled. The
// batch folder must already be protected by the caller. Every item is
// written to the manifest before its original is touched, and stays there
// (marked partial) if removing the original fails half-way, so nothing that
// was removed ever leaves the manifest.
func Quarantine(p *sys.Platform, root, program string, items []Leftover, now time.Time) (*Batch, error) {
	id := now.Format("20060102-150405") + "-" + strconv.FormatInt(now.UnixNano()%100000, 10)
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return nil, err
	}
	b := &Batch{ID: id, Program: program, At: now}
	if err := saveBatch(root, b, p.Key); err != nil {
		return nil, err
	}
	for i, it := range items {
		q := QItem{Kind: it.Kind, Original: it.Path, Value: it.Value}
		// record saves q in the manifest before the destructive step.
		recorded := false
		record := func() error {
			b.Items = append(b.Items, q)
			recorded = true
			return saveBatch(root, b, p.Key)
		}
		var err error
		switch it.Kind {
		case LeftDir, LeftFile:
			q.Stored = filepath.Join("data", strconv.Itoa(i))
			q.Bytes = it.Bytes
			if err = roomFor(p, it.Path, dir, it.Bytes); err != nil {
				break
			}
			// Marked partial while it moves: if Axion is closed or the PC
			// goes down in the middle, the manifest already says that the
			// files may be split between the two places (Restore merges).
			q.Partial = true
			if err = record(); err == nil {
				if err = movePath(it.Path, filepath.Join(dir, q.Stored)); err == nil {
					b.Items[len(b.Items)-1].Partial = false
				}
			}
		case LeftRegKey:
			var t *RegTree
			if t, err = ReadTree(p.Reg, it.Path); err == nil {
				q.Reg = t
				if err = record(); err == nil {
					err = DeleteTree(p.Reg, t)
				}
			}
		case LeftRunValue:
			var v sys.RegValue
			if v, err = p.Reg.GetValue(it.Path, it.Value); err == nil && !v.Exists {
				err = errors.New("a entrada não existe mais")
			}
			if err == nil {
				q.RegValue = &v
				if err = record(); err == nil {
					err = p.Reg.DeleteValue(it.Path, it.Value)
				}
			}
		case LeftTask:
			err = p.Tasks.SetEnabled(it.Path, false)
		default:
			err = fmt.Errorf("tipo desconhecido %q", it.Kind)
		}
		if err != nil {
			b.Errors = append(b.Errors, it.Label+": "+err.Error())
			if recorded {
				last := &b.Items[len(b.Items)-1]
				if touched(p, dir, last) {
					last.Partial = true
					b.Bytes += last.Bytes
				} else {
					b.Items = b.Items[:len(b.Items)-1]
				}
			}
			if err := saveBatch(root, b, p.Key); err != nil {
				return b, err
			}
			continue
		}
		if !recorded {
			b.Items = append(b.Items, q)
		}
		b.Bytes += q.Bytes
		if err := saveBatch(root, b, p.Key); err != nil {
			return b, err
		}
	}
	return b, saveBatch(root, b, p.Key)
}

// roomFor checks, before copying a folder to the quarantine on another
// drive, that the drive holding the quarantine has the space (with 1 GB to
// spare): filling the system drive would be worse than the leftover.
func roomFor(p *sys.Platform, src, dstDir string, bytes int64) error {
	sv, dv := strings.ToUpper(filepath.VolumeName(src)), strings.ToUpper(filepath.VolumeName(dstDir))
	if sv == dv || dv == "" || bytes <= 0 {
		return nil // same drive: a rename, no copy
	}
	_, free, err := p.Native.DiskSpace(dv + `\`)
	if err != nil {
		return nil // unknown: the copy itself fails cleanly if space runs out
	}
	if need := uint64(bytes) + 1<<30; free < need {
		return fmt.Errorf("sem espaço em %s para guardar uma cópia (%d MB necessários, %d MB livres); apague manualmente se quiser", dv, need>>20, free>>20)
	}
	return nil
}

// touched reports whether a failed item already changed something that only
// the manifest can undo: files that reached the quarantine folder, or a
// registry key or value that is no longer complete.
func touched(p *sys.Platform, dir string, q *QItem) bool {
	switch q.Kind {
	case LeftDir, LeftFile:
		return exists(filepath.Join(dir, q.Stored))
	case LeftRegKey:
		if q.Reg == nil {
			return false
		}
		t, err := ReadTree(p.Reg, q.Reg.Key)
		return err != nil || !sameTree(t, q.Reg)
	case LeftRunValue:
		v, err := p.Reg.GetValue(q.Original, q.Value)
		return err != nil || !v.Exists
	}
	return false
}

// sameTree compares the keys and value names of two trees.
func sameTree(a, b *RegTree) bool {
	if a == nil || b == nil || !strings.EqualFold(a.Key, b.Key) || len(a.Values) != len(b.Values) || len(a.Subs) != len(b.Subs) {
		return false
	}
	for name := range b.Values {
		if _, ok := a.Values[name]; !ok {
			return false
		}
	}
	subs := map[string]*RegTree{}
	for _, s := range a.Subs {
		subs[strings.ToLower(s.Key)] = s
	}
	for _, s := range b.Subs {
		if !sameTree(subs[strings.ToLower(s.Key)], s) {
			return false
		}
	}
	return true
}

// movePath renames src to dst, or copies and removes it across drives. Links
// inside a folder are moved as they are (never followed), and neither path
// may go through a link or junction: a program without administrator rights
// could swap a parent folder for a junction between the search and the move,
// and the move (done as administrator) would then act somewhere else.
func movePath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		return errors.New("é um link; não foi movido")
	}
	if err := noLinkAncestors(src); err != nil {
		return err
	}
	if err := noLinkAncestors(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	err = os.Rename(src, dst)
	if err == nil {
		return nil
	}
	// Copying is only for another drive. Any other failure (a file in use,
	// no permission) is reported: a copy plus removal there could fill the
	// drive or leave the item split between both places.
	if !crossDevice(err) {
		return fmt.Errorf("não foi possível mover (algum arquivo pode estar em uso): %w", err)
	}
	if err := copyTree(src, dst); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}

// noLinkAncestors refuses a path when a folder above it is a link or a
// junction.
func noLinkAncestors(p string) error {
	p = filepath.Clean(p)
	vol := filepath.VolumeName(p)
	for dir := filepath.Dir(p); len(dir) > len(vol)+1; dir = filepath.Dir(dir) {
		fi, err := os.Lstat(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return fmt.Errorf("%s é um link ou junção: nada foi movido", dir)
		}
	}
	return nil
}

// crossDevice reports a rename that failed because the target is on another
// drive (ERROR_NOT_SAME_DEVICE on Windows, EXDEV elsewhere).
func crossDevice(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	if runtime.GOOS == "windows" {
		return errno == 17
	}
	return errno == 18
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return errors.New("contém links: mova manualmente")
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// Batches lists the quarantine, newest first; Verified tells which batches
// carry a valid MAC for key.
func Batches(root string, key []byte) []Batch {
	entries, _ := os.ReadDir(root)
	var out []Batch
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(manifestPath(root, e.Name()))
		if err != nil {
			continue
		}
		var b Batch
		if json.Unmarshal(data, &b) == nil && b.ID == e.Name() {
			b.Verified = verifyBatch(key, &b)
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func validBatchID(id string) bool {
	if id == "" || len(id) > 40 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// Restore puts a batch back in place. Items whose original location is in
// use again are left in the quarantine and reported. Restore runs with
// administrator rights, so every item of the manifest is checked against
// what Quarantine can produce before anything is written.
func Restore(p *sys.Platform, root, id string) (*Batch, error) {
	if !validBatchID(id) {
		return nil, errors.New("lote inválido")
	}
	data, err := os.ReadFile(manifestPath(root, id))
	if err != nil {
		return nil, err
	}
	var b Batch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if !verifyBatch(p.Key, &b) {
		return nil, errors.New("este lote não tem a assinatura desta instalação do Axion (foi criado ou alterado fora dele); por segurança não será restaurado")
	}
	paths := p.Native.Paths()
	b.Errors = nil
	for i := len(b.Items) - 1; i >= 0; i-- {
		q := &b.Items[i]
		if q.Restored {
			continue
		}
		if err := validItem(q, paths); err != nil {
			b.Errors = append(b.Errors, q.Original+": "+err.Error())
			continue
		}
		var err error
		switch q.Kind {
		case LeftDir, LeftFile:
			stored := filepath.Join(root, id, filepath.Clean(q.Stored))
			switch {
			case !within(stored, filepath.Join(root, id, "data")):
				err = errors.New("caminho inválido no manifesto")
			case !exists(stored) && q.Partial:
				// Nothing reached the quarantine: the original stayed in place.
			case !exists(stored):
				err = errors.New("os arquivos da quarentena não foram encontrados")
			case exists(q.Original) && q.Partial:
				err = mergeBack(stored, q.Original)
			case exists(q.Original):
				err = errors.New("já existe algo no local original")
			default:
				err = movePath(stored, q.Original)
			}
			if err == nil && exists(q.Original) {
				// A move keeps the permissions inherited from the
				// quarantine folder (administrators only); the restored
				// folder takes those of its own parent again.
				if perr := p.Native.Reinherit(q.Original); perr != nil {
					b.Errors = append(b.Errors, q.Original+": restaurado, mas as permissões não puderam ser reajustadas ("+perr.Error()+"); se o programa acusar acesso negado, reinstale-o")
				}
			}
		case LeftRegKey:
			err = WriteTree(p.Reg, q.Reg)
		case LeftRunValue:
			err = p.Reg.SetValue(q.Original, q.Value, *q.RegValue)
		case LeftTask:
			err = p.Tasks.SetEnabled(q.Original, true)
		}
		if err != nil {
			b.Errors = append(b.Errors, q.Original+": "+err.Error())
			continue
		}
		q.Restored = true
	}
	b.Restored = len(b.Errors) == 0
	if err := saveBatch(root, &b, p.Key); err != nil {
		return &b, err
	}
	if b.Restored {
		_ = os.RemoveAll(filepath.Join(root, id))
	}
	return &b, nil
}

// validItem accepts only what FindLeftovers and Quarantine produce: folders
// specific to one program, registry keys of programs under Software (or
// their own Uninstall entry), values of the Run keys and scheduled tasks.
func validItem(q *QItem, paths sys.Paths) error {
	switch q.Kind {
	case LeftDir, LeftFile:
		if !specificDir(q.Original, paths, protectedDirs(paths), nil) {
			return errors.New("destino não permitido para restaurar")
		}
	case LeftRegKey:
		if q.Reg == nil || !strings.EqualFold(q.Reg.Key, q.Original) || !restorableKey(q.Original) {
			return errors.New("chave do Registro não permitida para restaurar")
		}
	case LeftRunValue:
		ok := false
		for _, src := range []string{startup.SrcRunUser, startup.SrcRunMachine, startup.SrcRunMachine32} {
			if strings.EqualFold(q.Original, runKeyOf(src)) {
				ok = true
			}
		}
		if !ok || q.Value == "" || q.RegValue == nil || (q.RegValue.Kind != sys.RegSZ && q.RegValue.Kind != sys.RegExpandSZ) {
			return errors.New("entrada de inicialização inválida")
		}
	case LeftTask:
		if !strings.HasPrefix(q.Original, `\`) {
			return errors.New("tarefa inválida")
		}
	default:
		return fmt.Errorf("tipo desconhecido %q", q.Kind)
	}
	return nil
}

// restorableKey: HKCU\Software\<name>[\...] or HKLM\SOFTWARE[\WOW6432Node]\<name>[\...]
// where <name> is not a Windows or policy area, or an entry directly under
// one of the Uninstall keys.
func restorableKey(key string) bool {
	k := strings.ToLower(strings.TrimRight(key, `\`))
	for _, r := range uninstallRoots {
		root := strings.ToLower(r.key) + `\`
		if strings.HasPrefix(k, root) && !strings.Contains(k[len(root):], `\`) && len(k) > len(root) {
			return true
		}
	}
	for _, root := range []string{`hkcu\software\`, `hklm\software\wow6432node\`, `hklm\software\`} {
		if !strings.HasPrefix(k, root) {
			continue
		}
		first, _, _ := strings.Cut(k[len(root):], `\`)
		switch first {
		case "", "microsoft", "classes", "policies", "wow6432node", "clients", "registeredapplications":
			return false
		}
		return usableName(first)
	}
	return false
}

// mergeBack returns files of a partially moved folder whose original is
// back (or never fully left): only what is missing at the original is
// moved, nothing there is overwritten, and the rest of the copy is dropped.
func mergeBack(stored, original string) error {
	err := filepath.WalkDir(stored, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(stored, p)
		target := filepath.Join(original, rel)
		if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return errors.New("contém links: restaure manualmente")
		}
		if d.IsDir() {
			if p == stored {
				return nil
			}
			if !exists(target) {
				if err := movePath(p, target); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			return nil
		}
		if !exists(target) {
			return movePath(p, target)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(stored)
}

// Delete removes a batch for good.
func Delete(root, id string) error {
	if !validBatchID(id) {
		return errors.New("lote inválido")
	}
	return os.RemoveAll(filepath.Join(root, id))
}

// Purge deletes batches older than Keep.
func Purge(root string, now time.Time) int {
	n := 0
	for _, b := range Batches(root, nil) {
		if now.Sub(b.At) > Keep && Delete(root, b.ID) == nil {
			n++
		}
	}
	return n
}
