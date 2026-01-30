package export

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/bitrise-io/go-utils/v2/fileutil"
)

// TODO:
// The extensions over the original fileutil.FileManager should be moved there in a separate ticket.

// SysStat holds file system stat information.
type SysStat struct {
	Uid int
	Gid int
}

// FileManager defines file management operations.
type FileManager interface {
	fileutil.FileManager

	CopyFile(src, dst string) error
	CopyFileFS(fsys fs.FS, src, dst string) error
	CopyDir(src, dst string) error
	CopyFS(dir string, fsys fs.FS) error

	Lchown(path string, uid, gid int) error
	CopyOwner(srcInfo os.FileInfo, dstPath string) error
	Chtimes(path string, atime, mtime time.Time) error
	CopyTimes(srcInfo os.FileInfo, dstPath string) error
	Chmod(path string, mode os.FileMode) error
	CopyMode(srcInfo os.FileInfo, dstPath string) error
	Lstat(path string) (os.FileInfo, error)
	Sys(info os.FileInfo) (SysStat, error)

	LastNLines(s string, n int) string
}

// NewFileManager creates a new FileManager instance.
func NewFileManager() FileManager {
	return &fileManager{wrapped: fileutil.NewFileManager()}
}

// fileManager implements FileManager interface.
type fileManager struct {
	wrapped fileutil.FileManager
}

// CopyFile copies a single file from src to dst.
func (fm *fileManager) CopyFile(src, dst string) error {
	srcDir := filepath.Dir(src)
	fsys := os.DirFS(srcDir)

	return fm.CopyFileFS(fsys, filepath.Base(src), dst)
}

// CopyFileFS is the excerpt from fs.CopyFS that copies a single file from fs.FS to dst path.
func (fm *fileManager) CopyFileFS(fsys fs.FS, src, dst string) error {
	r, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer r.Close() // nolint:errcheck
	info, err := r.Stat()
	if err != nil {
		return err
	}
	w, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}

	defer w.Close() // nolint:errcheck
	if _, err := io.Copy(w, r); err != nil {
		return &fs.PathError{Op: "Copy", Path: dst, Err: err}
	}
	if err := w.Sync(); err != nil {
		return &fs.PathError{Op: "Sync", Path: dst, Err: err}
	}
	if err := fm.CopyOwner(info, dst); err != nil {
		return &fs.PathError{Op: "CopyOwner", Path: dst, Err: err}
	}
	if err := fm.CopyMode(info, dst); err != nil {
		return &fs.PathError{Op: "CopyMode", Path: dst, Err: err}
	}
	if err := fm.CopyTimes(info, dst); err != nil {
		return &fs.PathError{Op: "CopyTimes", Path: dst, Err: err}
	}

	return nil
}

// CopyDir is a convenience method for copying a directory from src to dst.
// Note: symlinks are preserved during the copy operation
func (fm *fileManager) CopyDir(src, dst string) error {
	return fm.CopyFS(dst, os.DirFS(src))
}

// CopyFS is a local copy of fileutil.FileManager.CopyFS
//
// A copy of os.CopyFS because it messes up permissions when copying files
// from fs.FS
//
// CopyFS copies the file system fsys into the directory dir,
// creating dir if necessary.
//
// Preserves permissions and ownership when possible.
//
// CopyFS will not overwrite existing files. If a file name in fsys
// already exists in the destination, CopyFS will return an error
// such that errors.Is(err, fs.ErrExist) will be true.
//
// Symbolic links in dir are followed.
//
// New files added to fsys (including if dir is a subdirectory of fsys)
// while CopyFS is running are not guaranteed to be copied.
//
// Copying stops at and returns the first error encountered.
func (fm *fileManager) CopyFS(dir string, fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		fpath, err := filepath.Localize(path)
		if err != nil {
			return err
		}
		newPath := filepath.Join(dir, fpath)
		info, err := fs.Lstat(fsys, path)

		// This is not exhausetive in the original implementation either.
		// nolint:exhaustive
		switch d.Type() {
		case os.ModeDir:
			if err != nil {
				return err
			}
			if err := os.MkdirAll(newPath, 0777); err != nil {
				return err
			}
			if err := fm.CopyOwner(info, newPath); err != nil {
				return err
			}
			if err := fm.CopyMode(info, newPath); err != nil {
				return err
			}
			return fm.CopyTimes(info, newPath)

		case os.ModeSymlink:
			target, err := fs.ReadLink(fsys, path)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, newPath); err != nil {
				return err
			}
			if err := fm.CopyOwner(info, newPath); err != nil {
				return err
			}
			return fm.CopyTimes(info, newPath)

		case 0:
			return fm.CopyFileFS(fsys, path, newPath)

		default:
			return &os.PathError{Op: "CopyFS", Path: path, Err: os.ErrInvalid}
		}
	})
}

// Lchown ...
func (fm *fileManager) Lchown(path string, uid, gid int) error {
	return os.Lchown(path, uid, gid)
}

// CopyOwner invokes Lchown to copy ownership from srcInfo to dstPath.
func (fm *fileManager) CopyOwner(srcInfo os.FileInfo, dstPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	stat, ok := srcInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("missing Stat_t for symlink %s", dstPath)
	}
	// os.Lchown affects the link itself when given the link path
	if err := fm.Lchown(dstPath, int(stat.Uid), int(stat.Gid)); err != nil {
		return fmt.Errorf("lchown(symlink) %s: %w", dstPath, err)
	}
	return nil
}

// Chtimes ...
func (fm *fileManager) Chtimes(path string, atime, mtime time.Time) error {
	return os.Chtimes(path, atime, mtime)
}

// CopyTimes invokes Chtimes to copy access and modification times from srcInfo to dstPath.
func (fm *fileManager) CopyTimes(srcInfo os.FileInfo, dstPath string) error {
	if runtime.GOOS == "windows" {
		// On Windows we only set mod time (atime setting optional)
		if err := fm.Chtimes(dstPath, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
			// ignore or return depending on policy
			return fmt.Errorf("chtimes %s: %w", dstPath, err)
		}

	} else {
		if stat, ok := srcInfo.Sys().(*syscall.Stat_t); ok {
			// set times (for non-symlink paths we use os.Chtimes)
			if srcInfo.Mode()&os.ModeSymlink == 0 {
				atime := time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec)
				mtime := srcInfo.ModTime()
				if err := fm.Chtimes(dstPath, atime, mtime); err != nil {
					return fmt.Errorf("chtimes %s: %w", dstPath, err)
				}
			}
		}
	}
	return nil
}

// Chmod ...
func (fm *fileManager) Chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

// CopyMode invokes Chmod to copy file mode from srcInfo to dstPath.
func (fm *fileManager) CopyMode(srcInfo os.FileInfo, dstPath string) error {
	return fm.Chmod(dstPath, srcInfo.Mode())
}

// Lstat ...
func (fm *fileManager) Lstat(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

// Sys ...
func (fm *fileManager) Sys(info os.FileInfo) (SysStat, error) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return SysStat{Uid: int(stat.Uid), Gid: int(stat.Gid)}, nil
	}
	return SysStat{}, fmt.Errorf("failed to extract sys info for %s", info.Name())
}

// LastNLines returns the last n lines of the given string s.
func (fm *fileManager) LastNLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	// normalize CRLF to LF if needed
	if strings.Contains(s, "\r\n") {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}

	// skip trailing newlines so we don't count empty trailing lines
	i := len(s) - 1
	for i >= 0 && s[i] == '\n' {
		i--
	}
	if i < 0 {
		return "" // string was all newlines
	}

	// scan backward counting '\n' occurrences
	count := 0
	for ; i >= 0; i-- {
		if s[i] == '\n' {
			count++
			if count == n {
				// substring after this newline is the last n lines
				start := i + 1
				res := s[start:]
				// trim trailing whitespace (spaces, tabs, newlines, etc.)
				return strings.TrimRightFunc(res, func(r rune) bool {
					return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
				})
			}
		}
	}

	// fewer than n newlines => return whole string (trim trailing whitespace)
	return strings.TrimRightFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
	})
}

// ----------------------------------------------------------------

// Open - Implement FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) Open(path string) (*os.File, error) { return fm.wrapped.Open(path) }

// OpenReaderIfExists - Implement FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) OpenReaderIfExists(path string) (io.Reader, error) {
	return fm.wrapped.OpenReaderIfExists(path)
}

// ReadDirEntryNames - FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) ReadDirEntryNames(path string) ([]string, error) {
	return fm.wrapped.ReadDirEntryNames(path)
}

// Remove - Implement FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) Remove(path string) error { return fm.wrapped.Remove(path) }

// RemoveAll - FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) RemoveAll(path string) error { return fm.wrapped.RemoveAll(path) }

// Write - Implement FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) Write(path string, value string, perm os.FileMode) error {
	return fm.wrapped.Write(path, value, perm)
}

// WriteBytes - Implement FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) WriteBytes(path string, value []byte) error {
	return fm.wrapped.WriteBytes(path, value)
}

// FileSizeInBytes - FileManager methods by delegating to the wrapped FileManager.
func (fm *fileManager) FileSizeInBytes(pth string) (int64, error) {
	return fm.wrapped.FileSizeInBytes(pth)
}
