package main

import (
	"PikaFileService/connectors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/radovskyb/watcher"
)

//TODO: Log to file
func StartFWatch(folders []string, dstPath string) {
	w := watcher.New()
	w.SetMaxEvents(2)
	w.FilterOps(watcher.Rename, watcher.Move, watcher.Remove, watcher.Create, watcher.Write)
	r := regexp.MustCompile("(\\w|[-.])+$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				executeFilesystemOperation(event, dstPath)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	for _, file := range folders {
		if err := w.AddRecursive(file); err != nil {
			log.Fatalln(err)
		}
	}

	if err := w.Start(time.Millisecond * 500); err != nil {
		log.Fatalln(err)
	}
}

func executeFilesystemOperation(event watcher.Event, dstPath string) {
	switch {
	case event.Op == watcher.Create:
		dstPath = createDestinationPath(event.Path, dstPath)
		if !event.IsDir() {
			if err := connectors.CopyFile(event.Path, dstPath); err != nil {
				log.Println(err.Error())
			}
		} else {
			if cwd, _ := os.Getwd(); cwd != dstPath {
				if err := connectors.Mkdir(dstPath, event.Mode()); err != nil {
					log.Println(err.Error())
				}
			}
		}
	case event.Op == watcher.Rename:
		dstBeforeRename := createDestinationPath(event.OldPath, dstPath)
		dstPath = createDestinationPath(event.Path, dstPath)
		if err := connectors.RenameFile(dstPath, event.Path, dstBeforeRename); err != nil {
			log.Println(err.Error())
		}
	case event.Op == watcher.Remove:
		dstPath = createDestinationPath(event.OldPath, dstPath)
		if err := connectors.RemoveFile(dstPath); err != nil {
			log.Println(err.Error())
		}
	case event.Op == watcher.Write:
		dstPath = createDestinationPath(event.Path, dstPath)
		if !event.IsDir() {
			if err := connectors.CopyFile(event.Path, dstPath); err != nil {
				log.Println(err.Error())
			}
		}
	}
}

func createDestinationPath(path string, dstPath string) string {
	cwd, _ := os.Getwd()
	return filepath.Join(dstPath, strings.Replace(path, cwd, "", -1))
}
