package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

func main() {
	if len(os.Args) < 1 {
		panic("no command provided")
	}
	cmdString := os.Args[1]
	cmdArgs := os.Args[2:]
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()
	var files []string
	filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	for i := range files {
		if err := watcher.Add(files[i]); err != nil {
			panic(err)
		}
	}
	refresh := make(chan struct{}, 10)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op == fsnotify.Chmod {
					continue
				}
				fmt.Println(event.String())
				refresh <- struct{}{}
			}
		}
	}()

	for {
		cmd := exec.Command(cmdString, cmdArgs...)
		// Start porcess in seperate process group, so that it can be killed seperatly
		// in order to reclaim bound ports.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			panic(err)
		}
		scanner := bufio.NewScanner(io.MultiReader(stderr, stdout))
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		go func() {
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
		}()
		<-refresh

		// Get the procedss group id.
		pqgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			panic(err)
		}
		// Magic
		if err := syscall.Kill(-pqgid, 15); err != nil {
			panic(err)
		}
		fmt.Printf("\x1bc") //ANSI clear screen
	}
}
