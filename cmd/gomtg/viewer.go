package main

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/google/shlex"
)

var (
	imageViewerProcess *exec.Cmd
	imageViewerMutex   sync.Mutex
	imageViewerRunning bool
)

func spawnViewer(cmd string, autoReload bool, path string) error {
	if cmd == "" {
		return nil
	}

	if autoReload &&
		imageViewerProcess != nil && imageViewerProcess.Process != nil {
		var running bool
		imageViewerMutex.Lock()
		running = imageViewerRunning
		imageViewerMutex.Unlock()
		if running {
			return nil
		}
		imageViewerProcess = nil
	}

	parts, err := shlex.Split(cmd)
	if err != nil {
		return fmt.Errorf("%w: Invalid image viewer command: %s", err, cmd)
	}

	repl := false
	for i, arg := range parts {
		if !strings.Contains(arg, "{}") {
			continue
		}
		repl = true
		parts[i] = strings.ReplaceAll(arg, "{}", path)
	}

	if !repl {
		parts = append(parts, path)
	}

	killViewer()
	imageViewerProcess = exec.Command(parts[0], parts[1:]...)
	err = imageViewerProcess.Start()
	if err != nil {
		imageViewerProcess = nil
	}
	imageViewerMutex.Lock()
	imageViewerRunning = true
	imageViewerMutex.Unlock()

	go func() {
		imageViewerProcess.Wait()
		imageViewerMutex.Lock()
		imageViewerProcess = nil
		imageViewerRunning = false
		imageViewerMutex.Unlock()
	}()

	return err
}

func killViewer() {
	if imageViewerProcess != nil && imageViewerProcess.Process != nil {
		imageViewerProcess.Process.Kill()
	}
}
