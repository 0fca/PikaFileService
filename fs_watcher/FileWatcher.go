package fs_watcher

import (
	"log"
	"regexp"
	"time"

	"github.com/radovskyb/watcher"
	"PikaFileService/fs_connector"
)

//TODO: Log to file
func Main(folders []string) {
	w := watcher.New()
	w.SetMaxEvents(1)
	w.FilterOps(watcher.Rename, watcher.Move, watcher.Remove, watcher.Create)
	r := regexp.MustCompile("(\\w|[-.])+$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				executeFileOperation(event)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	for _, file := range folders{
		if err := w.Add(file); err != nil {
			log.Fatalln(err)
		}
	}

	go func() {
		w.Wait()
		w.TriggerEvent(watcher.Create, nil)
		w.TriggerEvent(watcher.Remove, nil)
	}()

	if err := w.Start(time.Millisecond * 500); err != nil {
		log.Fatalln(err)
	}
}

func executeFileOperation(event watcher.Event) {

	fs_connector.CopyFile()
	fs_connector.CopyFile(event.OldPath)
}
