package util

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// WriteTOMLAtomic encodes v as TOML and writes it to path atomically: it creates
// the parent directory with dirPerm, writes a sibling ".tmp" file with mode 0600,
// fsyncs it, then renames it over path. The final file therefore always has mode
// 0600 — suitable for secret material — and a crash never leaves a partially
// written target. On any failure the temp file is removed and the original path
// is left untouched.
//
// dirPerm lets the caller pick the parent-directory mode (e.g. 0700 for a
// directory holding private keys, 0755 for a less-sensitive registry). The temp
// and final file are always 0600 regardless.
//
// A failure that turns out to be about writability comes back wrapping
// ErrReadOnly and naming path, so callers do not have to re-diagnose an EROFS or
// an immutable flag themselves — see ClassifyWriteErr.
//
// This is the shared low-level primitive behind users.Manager.saveLocked and
// awg.Store.saveLocked; neither copy-pastes the open/sync/close/rename dance.
func WriteTOMLAtomic(path string, dirPerm os.FileMode, v any) error {
	return ClassifyWriteErr(path, writeTOMLAtomic(path, dirPerm, v))
}

// ClassifyWriteErr turns a failed write to path into ErrReadOnly when the path
// is in fact unwritable, and leaves every other failure alone: a full disk or an
// encoding bug is not something chmod fixes. The probe only runs on the failure
// path, so a healthy write never pays for it.
func ClassifyWriteErr(path string, err error) error {
	if err == nil || errors.Is(err, ErrReadOnly) {
		return err
	}
	if !PathWritable(path) {
		return ReadOnlyError(path)
	}
	return err
}

func writeTOMLAtomic(path string, dirPerm os.FileMode, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(v); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
