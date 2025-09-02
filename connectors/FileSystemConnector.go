package connectors

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"log"
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
	return err
}

func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	log.Println("Copying file from", src, "to", dst)
	if err != nil {
		return err
	}
	log.Println("Source file info:", sfi.Name(), sfi.Size(), sfi.Mode())
	if sfi.Mode().IsRegular() {
		log.Println("Source file is a regular file, can stat?")
		dfi, err := os.Stat(dst)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Println("Error stating destination file:", err)
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

		err = copyFileContents(src, dst)
		if err != nil {
			return err
		}
	}
	return err
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
