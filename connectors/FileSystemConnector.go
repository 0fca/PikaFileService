package connectors

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const syncLockFilename = "sync.lock"

func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //No need to handle it, it is deferred,
	// it will fire just after each file is already copied,
	//so it will be closed even if something fails on fs level
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
		return fmt.Errorf("RemoveFile: Couldnt delete regular file: %s", dst)
	}
	return
}

func RenameFile(oldPath string, newPath string, beforeRename string) (err error) {
	if filepath.VolumeName(newPath) == filepath.VolumeName(oldPath) {
		if err = os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("RenameFile: There was a problem while renaming a regular file %s to (%s)", oldPath, newPath)
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

func SyncDestinationFs(syncFolders []string, dstPath string, syncOffsetHours string) {
	syncLockFilePath := filepath.Join(dstPath, syncLockFilename)
	fi, _ := os.Stat(syncLockFilePath)
	if fi != nil {
		d, _ := time.ParseDuration(syncOffsetHours)
		if !time.Now().After(fi.ModTime().Add(d)) {
			log.Println("Omitting sync, " +
				"last sync was less than specified duration of time, this is a standard log, no action is required")
			return
		}
	}
	lfi, _ := os.Create(syncLockFilePath)
	err := lfi.Close()
	if err != nil {
		log.Fatal("There was a problem while creating a lock file in destination folder")
		return
	}
	for _, syncFolder := range syncFolders {
		fileList, err := os.Open(syncFolder)
		if err != nil {
			log.Fatal(err)
			return
		}
		files, err := fileList.Readdir(-1)
		for _, v := range files {
			if v.IsDir() {
				log.Println("Sync of subdirectories is not implemented at the moment, " +
					"this is just a standard log, no action is required")
			} else {
				_ = CopyFile(filepath.Join(syncFolder, v.Name()), filepath.Join(dstPath, v.Name()))
			}
		}
		// TODO: Implement fileList dump do JSON to sync.lock file.
	}
}
