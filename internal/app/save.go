package app

import (
	"os"
	"sync"
	"time"
)

// saveLocks serializes the writes of each state file: concurrent saves
// (two requests changing settings at once) used to share one ".tmp" file and
// fail or lose a change.
var saveLocks sync.Map // path → *sync.Mutex

// saveFile writes a state file atomically (temporary file + rename). The
// content is produced by snapshot inside the file's lock, so the last save
// always writes the newest state. The rename is retried briefly: on Windows
// an antivirus scanning the old file can hold it for a moment.
func saveFile(path string, snapshot func() ([]byte, error)) error {
	mu, _ := saveLocks.LoadOrStore(path, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	defer m.Unlock()
	b, err := snapshot()
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := writeSynced(tmp, b); err != nil {
		return err
	}
	for i := 0; ; i++ {
		err = os.Rename(tmp, path)
		if err == nil || i == 4 {
			break
		}
		time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// writeSynced writes the file and flushes it to the disk before the rename,
// so a power cut never leaves an empty or half-written settings file.
func writeSynced(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
