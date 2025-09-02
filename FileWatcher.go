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

// TODO: Log to file
func StartFWatch(folders []string, dstPath string, workDir string) {
	StartFWatchWithPikaCloud(folders, dstPath, workDir, &PikaCloudHandler{enabled: false})
}

func StartFWatchWithPikaCloud(folders []string, dstPath string, workDir string, pikaCloudHandler *PikaCloudHandler) {
	log.Println("Starting to watch folders:", folders)
	w := watcher.New()
	w.SetMaxEvents(10)
	w.FilterOps(watcher.Rename, watcher.Move, watcher.Remove, watcher.Create, watcher.Write)
	r := regexp.MustCompile(`(\w|[-.])+$`)
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				log.Println("File Event Detected:", event.Path)
				if pikaCloudHandler != nil && pikaCloudHandler.enabled {
					pikaCloudHandler.HandleFileOperation(event, dstPath, workDir)
				} else {
					executeFilesystemOperation(event, dstPath, workDir)
				}
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				log.Println("Watcher closed")
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

func executeFilesystemOperation(event watcher.Event, dstPath string, workDir string) {
	switch {
	case event.Op == watcher.Create:
		log.Println("File Creation Detected:", event.Path)
		dstPath = createDestinationPath(event.Path, dstPath, workDir)
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
		log.Println("File Rename Detected:", event.Path)
		dstBeforeRename := createDestinationPath(event.OldPath, dstPath, workDir)
		dstPath = createDestinationPath(event.Path, dstPath, workDir)
		if err := connectors.RenameFile(dstPath, event.Path, dstBeforeRename); err != nil {
			log.Println(err.Error())
		}
	case event.Op == watcher.Remove:
		log.Println("File Deletion Detected:", event.Path)
		dstPath = createDestinationPath(event.OldPath, dstPath, workDir)
		if err := connectors.RemoveFile(dstPath); err != nil {
			log.Println(err.Error())
		}
	case event.Op == watcher.Write:
		log.Println("File Modification Detected:", event.Path)
		dstPath = createDestinationPath(event.Path, dstPath, workDir)
		if !event.IsDir() {
			if err := connectors.CopyFile(event.Path, dstPath); err != nil {
				log.Println(err.Error())
			}
		}
	}
}

func createDestinationPath(path string, dstPath string, cwd string) string {
	log.Println("Current working directory:", cwd)
	log.Println("Destination path:", dstPath)
	log.Println("File path:", path)
	return filepath.Join(dstPath, strings.ReplaceAll(path, cwd, ""))
}
