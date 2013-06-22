// +build !appengine

package uio

import (
	"path/filepath"
	"time"

	"github.com/go-forks/fsnotify"

	ustr "github.com/metaleap/go-util/str"
)

//	A convenient wrapper around `go-forks/fsnotify.Watcher`.
//
//	Usage:
//		var w uio.Watcher
//		w.WatchIn(dir, pattern, runNow, handler)
//		go w.Go()
//		otherCode(laterOn...)
//		w.WatchIn(anotherDir...)
type Watcher struct {
	*fsnotify.Watcher

	//	Defaults to a `time.Duration` of 250 milliseconds
	DebounceNano int64

	//	A collection of custom `fsnotify.FileEvent` handlers.
	//	Not related to the handlers specified in your `Watcher.WatchIn()` calls.
	OnEvent []func(evt *fsnotify.FileEvent)

	//	A collection of custom `error` handlers.
	OnError []func(err error)

	dirsWatching map[string]bool

	allHandlers map[string][]WatcherHandler
}

//	Always returns a new `Watcher`, even if `err` is not `nil` (in which case, however, `me.Watcher` might be `nil`).
func NewWatcher() (me *Watcher, err error) {
	me = &Watcher{dirsWatching: map[string]bool{}, allHandlers: map[string][]WatcherHandler{}}
	me.DebounceNano = time.Duration(250 * time.Millisecond).Nanoseconds()
	me.Watcher, err = fsnotify.NewWatcher()
	return
}

//	Starts watching. A never-ending loop designed to be called in a new go-routine, as in `go myWatcher.Go()`.
func (me *Watcher) Go() {
	var (
		evt                            *fsnotify.FileEvent
		err                            error
		hasLast                        bool
		dif                            int64
		dirPath, dirPathAndNamePattern string
		on                             WatcherHandler
		ons                            []WatcherHandler
		onErr                          func(err error)
		onEvt                          func(evt *fsnotify.FileEvent)
	)
	lastEvt := map[string]int64{}
	for {
		select {
		case evt = <-me.Event:
			if evt != nil {
				_, hasLast = lastEvt[evt.Name]
				if dif = time.Now().UnixNano() - lastEvt[evt.Name]; dif > me.DebounceNano || !hasLast {
					for _, onEvt = range me.OnEvent {
						onEvt(evt)
					}
					dirPath = filepath.Dir(evt.Name)
					for dirPathAndNamePattern, ons = range me.allHandlers {
						if filepath.Dir(dirPathAndNamePattern) == dirPath && ustr.MatchesAny(filepath.Base(evt.Name), filepath.Base(dirPathAndNamePattern)) {
							for _, on = range ons {
								on(evt.Name)
							}
						}
					}
					lastEvt[evt.Name] = time.Now().UnixNano()
				}
			}
		case err = <-me.Error:
			if err != nil {
				for _, onErr = range me.OnError {
					onErr(err)
				}
			}
		}
	}
}

//	Watches dirs/files (whose `filepath.Base()` names match the specified `namePattern`) inside the specified `dirPath` for change event notifications.
//
//	`handler` is invoked whenever a change event is observed, providing the full path.
//
//	`runHandlerNow` allows immediate one-off invokation of `handler`. This will `DirWalker.Walk()` the `dirPath`.
//
//	An empty `namePattern` is equivalent to `*`.
func (me *Watcher) WatchIn(dirPath string, namePattern ustr.Pattern, runHandlerNow bool, handler WatcherHandler) (errs []error) {
	dirPath = filepath.Clean(dirPath)
	if _, ok := me.dirsWatching[dirPath]; !ok {
		if err := me.Watch(dirPath); err != nil {
			errs = append(errs, err)
		} else {
			me.dirsWatching[dirPath] = true
		}
	}
	if len(errs) == 0 {
		fullPath := filepath.Join(dirPath, string(namePattern))
		me.allHandlers[fullPath] = append(me.allHandlers[fullPath], handler)
		if runHandlerNow {
			errs = append(errs, watchRunHandler(dirPath, namePattern, handler)...)
		}
	}
	return
}
