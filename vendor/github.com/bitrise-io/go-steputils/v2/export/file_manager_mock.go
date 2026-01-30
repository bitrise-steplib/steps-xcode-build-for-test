package export

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"
	"time"
)

// MockFileManager is a simple test mock for FileManager.
type MockFileManager struct {
	// locking to support concurrent use in tests
	mu sync.Mutex

	// records
	OpenCalls               []string
	OpenReaderIfExistsCalls []string
	ReadDirEntryNamesCalls  []string
	RemoveCalls             []string
	RemoveAllCalls          []string
	WriteCalls              []struct {
		path, value string
		perm        os.FileMode
	}
	WriteBytesCalls []struct {
		path  string
		value []byte
	}
	FileSizeInBytesCalls []string

	CopyFileCalls   []struct{ src, dst string }
	CopyFileFSCalls []struct{ fsName, src, dst string } // fsName is informative
	CopyDirCalls    []struct{ src, dst string }
	CopyFSCalls     []struct {
		dir    string
		fsName string
	}
	LchownCalls []struct {
		path     string
		uid, gid int
	}
	CopyOwnerCalls []struct{ InfoName, dst string }
	ChtimesCalls   []struct {
		path         string
		atime, mtime time.Time
	}
	CopyTimesCalls []struct{ InfoName, dst string }
	ChmodCalls     []struct {
		path string
		mode os.FileMode
	}
	CopyModeCalls   []struct{ InfoName, dst string }
	LstatCalls      []string
	SysCalls        []string
	LastNLinesCalls []struct {
		s string
		n int
	}

	// configured returns (default zero values mean no special behavior)
	OpenFunc               func(path string) (*os.File, error)
	OpenReaderIfExistsFunc func(path string) (io.Reader, error)
	ReadDirEntryNamesFunc  func(path string) ([]string, error)
	RemoveFunc             func(path string) error
	RemoveAllFunc          func(path string) error
	WriteFunc              func(path string, value string, perm os.FileMode) error
	WriteBytesFunc         func(path string, value []byte) error
	FileSizeInBytesFunc    func(path string) (int64, error)

	CopyFileFunc   func(src, dst string) error
	CopyFileFSFunc func(fsys fs.FS, src, dst string) error
	CopyDirFunc    func(src, dst string) error
	CopyFSFunc     func(dir string, fsys fs.FS) error
	LchownFunc     func(path string, uid, gid int) error
	CopyOwnerFunc  func(srcInfo os.FileInfo, dstPath string) error
	ChtimesFunc    func(path string, atime, mtime time.Time) error
	CopyTimesFunc  func(srcInfo os.FileInfo, dstPath string) error
	ChmodFunc      func(path string, mode os.FileMode) error
	CopyModeFunc   func(srcInfo os.FileInfo, dstPath string) error
	LstatFunc      func(path string) (os.FileInfo, error)
	SysFunc        func(info os.FileInfo) (SysStat, error)
	LastNLinesFunc func(s string, n int) string

	// canned returns for Lstat/Sys (optional convenience)
	LstatReturnFileInfo os.FileInfo
	LstatReturnError    error
	SysReturn           SysStat
	SysReturnError      error
}

// Ensure MockFileManager implements FileManager
var _ FileManager = (*MockFileManager)(nil)

// helpers to record calls and delegate to configured funcs (or simple defaults)

func (m *MockFileManager) record(muFunc func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	muFunc()
}

// Open ...
func (m *MockFileManager) Open(path string) (*os.File, error) {
	m.record(func() { m.OpenCalls = append(m.OpenCalls, path) })
	if m.OpenFunc != nil {
		return m.OpenFunc(path)
	}
	return nil, errors.New("Open not implemented in mock")
}

// OpenReaderIfExists ...
func (m *MockFileManager) OpenReaderIfExists(path string) (io.Reader, error) {
	m.record(func() { m.OpenReaderIfExistsCalls = append(m.OpenReaderIfExistsCalls, path) })
	if m.OpenReaderIfExistsFunc != nil {
		return m.OpenReaderIfExistsFunc(path)
	}
	return nil, nil
}

// ReadDirEntryNames ...
func (m *MockFileManager) ReadDirEntryNames(path string) ([]string, error) {
	m.record(func() { m.ReadDirEntryNamesCalls = append(m.ReadDirEntryNamesCalls, path) })
	if m.ReadDirEntryNamesFunc != nil {
		return m.ReadDirEntryNamesFunc(path)
	}
	return nil, nil
}

// Remove ...
func (m *MockFileManager) Remove(path string) error {
	m.record(func() { m.RemoveCalls = append(m.RemoveCalls, path) })
	if m.RemoveFunc != nil {
		return m.RemoveFunc(path)
	}
	return nil
}

// RemoveAll ...
func (m *MockFileManager) RemoveAll(path string) error {
	m.record(func() { m.RemoveAllCalls = append(m.RemoveAllCalls, path) })
	if m.RemoveAllFunc != nil {
		return m.RemoveAllFunc(path)
	}
	return nil
}

// Write ...
func (m *MockFileManager) Write(path string, value string, perm os.FileMode) error {
	m.record(func() {
		m.WriteCalls = append(m.WriteCalls, struct {
			path, value string
			perm        os.FileMode
		}{path, value, perm})
	})
	if m.WriteFunc != nil {
		return m.WriteFunc(path, value, perm)
	}
	return nil
}

// WriteBytes ...
func (m *MockFileManager) WriteBytes(path string, value []byte) error {
	m.record(func() {
		m.WriteBytesCalls = append(m.WriteBytesCalls, struct {
			path  string
			value []byte
		}{path, append([]byte(nil), value...)})
	})
	if m.WriteBytesFunc != nil {
		return m.WriteBytesFunc(path, value)
	}
	return nil
}

// FileSizeInBytes ...
func (m *MockFileManager) FileSizeInBytes(pth string) (int64, error) {
	m.record(func() { m.FileSizeInBytesCalls = append(m.FileSizeInBytesCalls, pth) })
	if m.FileSizeInBytesFunc != nil {
		return m.FileSizeInBytesFunc(pth)
	}
	return 0, nil
}

// CopyFile ...
func (m *MockFileManager) CopyFile(src, dst string) error {
	m.record(func() { m.CopyFileCalls = append(m.CopyFileCalls, struct{ src, dst string }{src, dst}) })
	if m.CopyFileFunc != nil {
		return m.CopyFileFunc(src, dst)
	}
	return nil
}

// CopyFileFS ...
func (m *MockFileManager) CopyFileFS(fsys fs.FS, src, dst string) error {
	// try to determine fs name for recording
	name := "<fs>"
	if dirFS, ok := fsys.(interface{ String() string }); ok {
		name = dirFS.String()
	}
	m.record(func() {
		m.CopyFileFSCalls = append(m.CopyFileFSCalls, struct{ fsName, src, dst string }{name, src, dst})
	})
	if m.CopyFileFSFunc != nil {
		return m.CopyFileFSFunc(fsys, src, dst)
	}
	return nil
}

// CopyDir ...
func (m *MockFileManager) CopyDir(src, dst string) error {
	m.record(func() { m.CopyDirCalls = append(m.CopyDirCalls, struct{ src, dst string }{src, dst}) })
	if m.CopyDirFunc != nil {
		return m.CopyDirFunc(src, dst)
	}
	return nil
}

// CopyFS ...
func (m *MockFileManager) CopyFS(dir string, fsys fs.FS) error {
	name := "<fs>"
	if dirFS, ok := fsys.(interface{ String() string }); ok {
		name = dirFS.String()
	}
	m.record(func() {
		m.CopyFSCalls = append(m.CopyFSCalls, struct {
			dir    string
			fsName string
		}{dir, name})
	})
	if m.CopyFSFunc != nil {
		return m.CopyFSFunc(dir, fsys)
	}
	return nil
}

// Lchown ...
func (m *MockFileManager) Lchown(path string, uid, gid int) error {
	m.record(func() {
		m.LchownCalls = append(m.LchownCalls, struct {
			path     string
			uid, gid int
		}{path, uid, gid})
	})
	if m.LchownFunc != nil {
		return m.LchownFunc(path, uid, gid)
	}
	return nil
}

// CopyOwner ...
func (m *MockFileManager) CopyOwner(srcInfo os.FileInfo, dstPath string) error {
	name := "<info>"
	if srcInfo != nil {
		name = srcInfo.Name()
	}
	m.record(func() { m.CopyOwnerCalls = append(m.CopyOwnerCalls, struct{ InfoName, dst string }{name, dstPath}) })
	if m.CopyOwnerFunc != nil {
		return m.CopyOwnerFunc(srcInfo, dstPath)
	}
	// default: try to extract uid/gid from srcInfo.Sys() if it's a syscall.Stat_t-like struct,
	// otherwise do nothing
	if srcInfo == nil {
		return nil
	}
	if m.SysFunc != nil {
		if ss, err := m.SysFunc(srcInfo); err == nil {
			_ = ss // ignore by default
		}
	}
	return nil
}

// Chtimes ...
func (m *MockFileManager) Chtimes(path string, atime, mtime time.Time) error {
	m.record(func() {
		m.ChtimesCalls = append(m.ChtimesCalls, struct {
			path         string
			atime, mtime time.Time
		}{path, atime, mtime})
	})
	if m.ChtimesFunc != nil {
		return m.ChtimesFunc(path, atime, mtime)
	}
	return nil
}

// CopyTimes ...
func (m *MockFileManager) CopyTimes(srcInfo os.FileInfo, dstPath string) error {
	name := "<info>"
	if srcInfo != nil {
		name = srcInfo.Name()
	}
	m.record(func() { m.CopyTimesCalls = append(m.CopyTimesCalls, struct{ InfoName, dst string }{name, dstPath}) })
	if m.CopyTimesFunc != nil {
		return m.CopyTimesFunc(srcInfo, dstPath)
	}
	return nil
}

// Chmod ...
func (m *MockFileManager) Chmod(path string, mode os.FileMode) error {
	m.record(func() {
		m.ChmodCalls = append(m.ChmodCalls, struct {
			path string
			mode os.FileMode
		}{path, mode})
	})
	if m.ChmodFunc != nil {
		return m.ChmodFunc(path, mode)
	}
	return nil
}

// CopyMode ...
func (m *MockFileManager) CopyMode(srcInfo os.FileInfo, dstPath string) error {
	name := "<info>"
	if srcInfo != nil {
		name = srcInfo.Name()
	}
	m.record(func() { m.CopyModeCalls = append(m.CopyModeCalls, struct{ InfoName, dst string }{name, dstPath}) })
	if m.CopyModeFunc != nil {
		return m.CopyModeFunc(srcInfo, dstPath)
	}
	return nil
}

// Lstat ...
func (m *MockFileManager) Lstat(path string) (os.FileInfo, error) {
	m.record(func() { m.LstatCalls = append(m.LstatCalls, path) })
	if m.LstatFunc != nil {
		return m.LstatFunc(path)
	}
	// default: return configured canned FileInfo or error
	if m.LstatReturnError != nil {
		return nil, m.LstatReturnError
	}
	if m.LstatReturnFileInfo != nil {
		return m.LstatReturnFileInfo, nil
	}
	return nil, os.ErrNotExist
}

// Sys ...
func (m *MockFileManager) Sys(info os.FileInfo) (SysStat, error) {
	m.record(func() { m.SysCalls = append(m.SysCalls, info.Name()) })
	if m.SysFunc != nil {
		return m.SysFunc(info)
	}
	if m.SysReturnError != nil {
		return SysStat{}, m.SysReturnError
	}
	return m.SysReturn, nil
}

// LastNLines ...
func (m *MockFileManager) LastNLines(s string, n int) string {
	m.record(func() {
		m.LastNLinesCalls = append(m.LastNLinesCalls, struct {
			s string
			n int
		}{s, n})
	})
	if m.LastNLinesFunc != nil {
		return m.LastNLinesFunc(s, n)
	}
	// default behavior: tail-like
	parts := bytes.Split([]byte(s), []byte("\n"))
	if n <= 0 || len(parts) == 0 {
		return ""
	}
	if len(parts) <= n {
		return string(bytes.TrimRight(parts[len(parts)-1], " \t\r\n"))
	}
	last := bytes.Join(parts[len(parts)-n:], []byte("\n"))
	return string(bytes.TrimRight(last, " \t\r\n"))
}
