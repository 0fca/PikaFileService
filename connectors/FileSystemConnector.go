package connectors

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}

func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if sfi.Mode().IsRegular() {
		dfi, err := os.Stat(dst)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			if !(dfi.Mode().IsRegular()) {
				return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
			}
			if os.SameFile(sfi, dfi) {
				return fmt.Errorf("%s is not the same as %s", sfi.Name(), dfi.Name())
			}
		}
		if err = os.Link(src, dst); err == nil {
			return err
		}
		err = copyFileContents(src, dst)
	}
	return
}

func RemoveFile(dst string) (err error) {
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return err
	} else {
		if dfi.IsDir() {
			fmt.Println("This is a directory, this is just a standard log, no action is required")
		}
	}

	if err = os.RemoveAll(dst); err != nil {
		return fmt.Errorf("Couldnt delete regular file: %s", dst)
	}
	return
}

func RenameFile(oldPath string, newPath string, beforeRename string) (err error) {
	if filepath.VolumeName(newPath) == filepath.VolumeName(oldPath) {
		if err = os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("There was a problem while renaming a regular file")
		}
	} else {
		if err = RemoveFile(beforeRename); err != nil {
			return err
		}
		if err = CopyFile(newPath, oldPath); err != nil {
			return err
		}
	}
	return
}

func Mkdir(dirPath string, perms os.FileMode) (err error) {
	if err := os.Mkdir(dirPath, perms); err != nil {
		return err
	}
	return
}
