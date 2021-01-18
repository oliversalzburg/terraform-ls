package watcher

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/terraform-ls/internal/terraform/datadir"
	"github.com/hashicorp/terraform-ls/internal/terraform/module"
)

// Watcher is a wrapper around native fsnotify.Watcher
// It provides the ability to detect actual file changes
// (rather than just events that may not be changing any bytes)
type watcher struct {
	fw          *fsnotify.Watcher
	modMgr      module.ModuleManager
	modulePaths map[string]bool
	logger      *log.Logger

	watching   bool
	cancelFunc context.CancelFunc
}

type WatcherFactory func() (Watcher, error)

func NewWatcher(modMgr module.ModuleManager) (Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &watcher{
		fw:          fw,
		modMgr:      modMgr,
		logger:      defaultLogger,
		modulePaths: make(map[string]bool, 0),
	}, nil
}

var defaultLogger = log.New(ioutil.Discard, "", 0)

func (w *watcher) SetLogger(logger *log.Logger) {
	w.logger = logger
}

func (w *watcher) AddModule(path string, dir *datadir.DataDir) error {
	if dir == nil {
		return fmt.Errorf("%s: no datadir provided for module", path)
	}

	path = filepath.Clean(path)
	w.modulePaths[path] = true

	err := w.fw.Add(path)
	if err != nil {
		return err
	}

	if dir.ModuleManifestPath != "" {
		err := w.fw.Add(dir.ModuleManifestPath)
		if err != nil {
			return err
		}
	}
	if dir.PluginLockFilePath != "" {
		err := w.fw.Add(dir.PluginLockFilePath)
		if err != nil {
			return err
		}
	}

	return err
}

func (w *watcher) run(ctx context.Context) {
	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				// TODO
				w.modMgr.EnqueueModuleOp(dir, opType)
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				// TODO
				w.modMgr.EnqueueModuleOp(dir, opType)
			}

			if event.Op&fsnotify.Remove == fsnotify.Remove {
				// TODO: remove module-related individual paths
			}
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			w.logger.Println("watch error:", err)
		}
	}
}

func (w *watcher) Start() error {
	if w.watching {
		w.logger.Println("watching already in progress")
		return nil
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	w.cancelFunc = cancelFunc
	w.watching = true

	w.logger.Printf("watching for changes ...")
	go w.run(ctx)

	return nil
}

func (w *watcher) Stop() error {
	if !w.watching {
		return nil
	}

	w.cancelFunc()

	err := w.fw.Close()
	if err == nil {
		w.watching = false
	}

	return err
}
