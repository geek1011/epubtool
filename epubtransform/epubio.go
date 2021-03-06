package epubtransform

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pgaskin/epubtool/util"
)

// AutoInput automatically chooses an InputFunc from FileInput and DirInput.
func AutoInput(path string) InputFunc {
	return func(epubdir string) error {
		if fi, err := os.Stat(path); err != nil {
			return util.Wrap(err, "could not stat input")
		} else if fi.IsDir() {
			return DirInput(path)(epubdir)
		} else if filepath.Ext(path) == ".epub" {
			return FileInput(path)(epubdir)
		}
		return errors.New("unrecognized input file")
	}
}

// AutoOutput automatically chooses an OutputFunc to be the same as the input.
func AutoOutput(inputPath string) OutputFunc {
	return func(epubdir string) error {
		if fi, err := os.Stat(inputPath); err != nil {
			return util.Wrap(err, "could not stat input")
		} else if fi.IsDir() {
			return replaceOutputWrapper(inputPath, DirOutput)(epubdir)
		} else if filepath.Ext(inputPath) == ".epub" {
			return replaceOutputWrapper(inputPath, FileOutput)(epubdir)
		}
		return errors.New("unrecognized input file")
	}
}

// replaceOutputWrapper wraps a path-based OutputFunc generator to allow overwriting an existing output safely.
func replaceOutputWrapper(outputPath string, fn func(path string) OutputFunc) OutputFunc {
	return func(epubdir string) error {
		td, err := ioutil.TempDir("", "epubio-*")
		if err != nil {
			return util.Wrap(err, "error creating temp output dir")
		}
		defer os.RemoveAll(td)

		tdo := filepath.Join(td, filepath.Base(outputPath))
		if err := fn(tdo)(epubdir); err != nil {
			return err
		}

		os.RemoveAll(outputPath)

		if err := util.Copy(tdo, outputPath); err != nil {
			return util.Wrap(err, "error copying output into place")
		}
		return nil
	}
}

// TODO: reduce duplication between AutoInput and AutoOutput

// FileInput returns an InputFunc to read from an epub file.
func FileInput(file string) InputFunc {
	return func(epubdir string) error {
		os.RemoveAll(epubdir)
		return util.Unzip(file, epubdir)
	}
}

// DirOutput returns an OutputFunc to write to a directory. The destination must not exist.
func DirOutput(dir string) OutputFunc {
	return func(epubdir string) error {
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("output directory %#v already exists", dir)
		}
		return util.CopyDir(epubdir, dir)
	}
}

// DirInput returns an InputFunc to read from an unpacked epub directory.
func DirInput(dir string) InputFunc {
	return func(epubdir string) error {
		os.RemoveAll(epubdir)
		return util.CopyDir(dir, epubdir)
	}
}

// FileOutput returns an OutputFunc write to a epub file. The destination must not exist.
func FileOutput(file string) OutputFunc {
	return func(epubdir string) error {
		f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return util.Wrap(err, "error creating destination file")
		}
		defer f.Close()

		zw := zip.NewWriter(f)
		defer zw.Close()

		if mimetypeWriter, err := zw.CreateHeader(&zip.FileHeader{
			Name:   "mimetype",
			Method: zip.Store, // Do not compress mimetype
		}); err != nil {
			return util.Wrap(err, "error writing mimetype to epub")
		} else if _, err = mimetypeWriter.Write([]byte("application/epub+zip")); err != nil {
			return util.Wrap(err, "error writing mimetype to epub")
		}

		if err := filepath.Walk(epubdir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(epubdir, path)
			if err != nil {
				return fmt.Errorf("error getting relative path of %#v", path)
			}

			// Skip if it is trying to pack itself, is not regular file, or is mimetype
			if path == epubdir || !info.Mode().IsRegular() || filepath.Base(path) == "mimetype" {
				return nil
			}

			fw, err := zw.Create(relPath)
			if err != nil {
				return util.Wrap(err, `error creating file %#v in epub`, relPath)
			}

			sf, err := os.Open(path)
			if err != nil {
				return util.Wrap(err, "error reading file %#v", path)
			}
			defer sf.Close()

			if _, err := io.Copy(fw, sf); err != nil {
				return util.Wrap(err, "error writing file %#v to epub", relPath)
			}

			return nil
		}); err != nil {
			return util.Wrap(err, "error creating epub")
		}

		return nil
	}
}
