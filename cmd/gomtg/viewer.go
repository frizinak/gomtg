package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/google/shlex"
)

var (
	imageViewerProcess *exec.Cmd
	imageViewerMutex   sync.Mutex
	imageViewerRunning bool
	imageViewerPID     int
)

func spawnViewer(cmd, refreshCmd string, autoReload bool, path string) error {
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

	var refresh *exec.Cmd
	if !autoReload {
		parts, err := shlex.Split(refreshCmd)
		if err != nil {
			return fmt.Errorf("%w: Invalid image viewer refresh command: %s", err, refreshCmd)
		}

		imageViewerMutex.Lock()
		running := imageViewerRunning
		pid := imageViewerPID
		imageViewerMutex.Unlock()
		if running {
			for i, arg := range parts {
				arg = strings.ReplaceAll(arg, "{pid}", strconv.Itoa(pid))
				arg = strings.ReplaceAll(arg, "{fn}", path)
				parts[i] = arg
			}
			refresh = exec.Command(parts[0], parts[1:]...)
			return refresh.Run()
		}
	}

	parts, err := shlex.Split(cmd)
	if err != nil {
		return fmt.Errorf("%w: Invalid image viewer command: %s", err, cmd)
	}

	repl := false
	for i, arg := range parts {
		if strings.Contains(arg, "{fn}") {
			repl = true
			parts[i] = strings.ReplaceAll(arg, "{fn}", path)
		}
	}

	if !repl {
		parts = append(parts, path)
	}

	killViewer()
	imageViewerProcess = exec.Command(parts[0], parts[1:]...)

	// prevent process from being killed by ctrl-c in controlling terminal
	imageViewerProcess.SysProcAttr = sysProcAddr()

	err = imageViewerProcess.Start()
	if err != nil {
		imageViewerProcess = nil
	}
	imageViewerMutex.Lock()
	imageViewerRunning = true
	imageViewerPID = 0
	if imageViewerProcess.Process != nil {
		imageViewerPID = imageViewerProcess.Process.Pid
	}
	imageViewerMutex.Unlock()

	go func() {
		_ = imageViewerProcess.Wait()
		imageViewerMutex.Lock()
		imageViewerProcess = nil
		imageViewerRunning = false
		imageViewerMutex.Unlock()
	}()

	return err
}

func killViewer() {
	if imageViewerProcess != nil && imageViewerProcess.Process != nil {
		_ = imageViewerProcess.Process.Kill()
	}
}
