package fs_watcher

import (
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/radovskyb/watcher"
)

func main(folders []string) {
	w := watcher.New()
	w.SetMaxEvents(1)
	w.FilterOps(watcher.Rename, watcher.Move, watcher.Remove, watcher.Create)
	r := regexp.MustCompile("^abc$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				fmt.Println(event) // Print the event's info.
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

	for path, f := range w.WatchedFiles() {
		//TODO: Log to file
		fmt.Printf("%s: %s\n", path, f.Name())
	}

	fmt.Println()

	go func() {
		w.Wait()
		w.TriggerEvent(watcher.Create, nil)
		w.TriggerEvent(watcher.Remove, nil)
	}()

	// Start the watching process - it'll check for changes every 500ms.
	if err := w.Start(time.Millisecond * 500); err != nil {
		//TODO: Do this to log into log file.
		log.Fatalln(err)
	}
}
