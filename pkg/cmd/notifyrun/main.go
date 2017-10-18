package main

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/fsnotify/fsnotify"
	"github.com/google/shlex"
	"gopkg.in/urfave/cli.v1"
)

const (
	execFlag   string = "exec"
	ignoreFlag string = "ignore"
)

func main() {
	app := cli.NewApp()
	app.Usage = "notifyrun"

	app.Flags = []cli.Flag{
		cli.StringFlag{Name: execFlag, Usage: "Command to exec"},
		cli.StringSliceFlag{Name: ignoreFlag, Usage: "Files to ignore"},
	}

	app.Action = defaultAction

	app.RunAndExitOnError()
}

func defaultAction(c *cli.Context) error {
	execStr := c.GlobalString(execFlag)
	ignoreStrSlice := c.GlobalStringSlice(ignoreFlag)
	args := append([]string{c.Args().First()}, c.Args().Tail()...)

	if execStr != "" {
		return execAction(execStr, ignoreStrSlice, args)
	}
	return fmt.Errorf("must select an action type")
}

func execAction(execStr string, ignoreStrSlice, files []string) error {
	if len(files) == 0 {
		return fmt.Errorf("must specify files/directories to watch")
	}

	splitExecStr, err := shlex.Split(execStr)
	if err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	errC := make(chan error)
	go handleExecEvents(watcher, splitExecStr, ignoreStrSlice, errC)

	log.Print("watching:", files)
	for _, f := range files {
		if err := watcher.Add(f); err != nil {
			return err
		}
	}
	return <-errC
}

func handleExecEvents(w *fsnotify.Watcher, splitExecStr, ignores []string, errC chan error) {
	for {
		if term, err := handleExecEventOnce(w, splitExecStr, ignores); err != nil {
			errC <- err
			return
		} else if term {
			errC <- nil
			return
		}
	}
}

func handleExecEventOnce(w *fsnotify.Watcher, splitExecStr, ignores []string) (bool, error) {
	ignoreMap := make(map[string]struct{})
	for _, s := range ignores {
		ignoreMap[s] = struct{}{}
	}

	select {
	case event := <-w.Events:
		log.Print("event:", event)
		if _, ok := ignoreMap[event.Name]; ok {
			log.Print("ignore event:", event)
			return false, nil
		}

		cmd := exec.Command(splitExecStr[0], splitExecStr[1:]...)
		outB, err := cmd.CombinedOutput()
		if err != nil {
			return false, err
		}
		log.Print("cmd:", string(outB))
	case err := <-w.Errors:
		return false, err
	}
	return false, nil
}
