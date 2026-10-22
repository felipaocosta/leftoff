package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrLocked means another LeftOff has this data folder open. Two
// processes writing one library.json overwrite each other's changes (and
// an older version would rewrite a newer schema), so the second one stops.
var ErrLocked = errors.New("the data folder is open in another LeftOff")

const lockFile = "library.lock"

// DirLock holds the data folder for this process (RULES §7). The operating
// system drops it when the process ends, even after a crash.
type DirLock struct {
	f      *os.File
	holder Holder
}

// Holder is who has a data folder, as its lock file says.
type Holder struct {
	PID   int
	Owner string // mode and version, e.g. "desktop 8b326ae"
	URL   string // its local server once it listens, e.g. http://127.0.0.1:8732/
}

func (h Holder) String() string {
	if h.PID == 0 {
		return ""
	}
	return fmt.Sprintf("PID %d, %s", h.PID, h.Owner)
}

// LockDir takes the data folder for this process; owner (mode and
// version) is written down for the message another process shows.
func LockDir(dir, owner string) (*DirLock, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, lockFile)
	f, err := lockExclusive(path)
	if errors.Is(err, ErrLocked) {
		if h, herr := ReadHolder(dir); herr == nil && h.PID != 0 {
			return nil, fmt.Errorf("%w (%s): %s", ErrLocked, h, dir)
		}
		return nil, fmt.Errorf("%w: %s", ErrLocked, dir)
	}
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	l := &DirLock{f: f, holder: Holder{PID: os.Getpid(), Owner: owner}}
	l.write() // best effort: the lock holds even without the note
	return l, nil
}

// SetURL writes down where this process's server listens, so a second
// LeftOff can ask it to quit (or to show itself).
func (l *DirLock) SetURL(url string) {
	l.holder.URL = url
	l.write()
}

func (l *DirLock) write() {
	if l.f.Truncate(0) != nil {
		return
	}
	_, _ = l.f.WriteAt([]byte(fmt.Sprintf("pid=%d\nowner=%s\nurl=%s\n", l.holder.PID, l.holder.Owner, l.holder.URL)), 0) // a note only
}

// LockPath is the lock file of a data folder (to find its holder by the
// open file, when the note is missing).
func LockPath(dir string) string { return filepath.Join(dir, lockFile) }

// ReadHolder reads who has the data folder (zero when nobody wrote it).
func ReadHolder(dir string) (Holder, error) {
	b, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return Holder{}, fmt.Errorf("read lock: %w", err)
	}
	var h Holder
	text := strings.TrimSpace(string(b))
	// The first version of the lock wrote one line: "PID 1546125, app 8b326ae".
	if rest, ok := strings.CutPrefix(text, "PID "); ok {
		pid, owner, _ := strings.Cut(rest, ", ")
		h.PID, _ = strconv.Atoi(pid) // a bad number reads as nobody
		h.Owner = owner
		return h, nil
	}
	for line := range strings.SplitSeq(text, "\n") {
		key, value, _ := strings.Cut(line, "=")
		switch key {
		case "pid":
			h.PID, _ = strconv.Atoi(value) // a bad number reads as nobody
		case "owner":
			h.Owner = value
		case "url":
			h.URL = value
		}
	}
	return h, nil
}

// Release gives the folder back (closing the file drops the lock).
func (l *DirLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	if err != nil {
		return fmt.Errorf("release data dir lock: %w", err)
	}
	return nil
}
