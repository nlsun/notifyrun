package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/shlex"
	"gopkg.in/urfave/cli.v1"
)

const (
	execFlag        string = "exec"
	ignoreFlag      string = "ignore"
	ignoreEventFlag string = "ignoreEvent"
)

func main() {
	app := cli.NewApp()
	app.Usage = "notifyrun"

	app.Flags = []cli.Flag{
		cli.StringFlag{Name: execFlag, Usage: "Command to exec"},
		cli.StringSliceFlag{Name: ignoreFlag, Usage: "Files to ignore"},
		cli.StringSliceFlag{Name: ignoreEventFlag, Usage: "Events to ignore"},
	}

	app.Action = defaultAction

	app.RunAndExitOnError()
}

func defaultAction(c *cli.Context) error {
	execStr := c.GlobalString(execFlag)
	ignoreStrSlice := c.GlobalStringSlice(ignoreFlag)
	ignoreEventStrSlice := c.GlobalStringSlice(ignoreEventFlag)
	args := append([]string{c.Args().First()}, c.Args().Tail()...)

	if execStr != "" {
		return execAction(execStr, ignoreStrSlice, ignoreEventStrSlice, args)
	}
	return fmt.Errorf("must select an action type")
}

func execAction(execStr string, ignoreStrSlice, ignoreEventStrSlice, files []string) error {
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
	go handleExecEvents(watcher, splitExecStr, ignoreStrSlice, ignoreEventStrSlice, errC)

	log.Print("watching: ", files)
	for _, f := range files {
		if err := watcher.Add(f); err != nil {
			return err
		}
	}
	return <-errC
}

func handleExecEvents(w *fsnotify.Watcher, splitExecStr, ignores, ignoreEvents []string, errC chan error) {
	forceEventC := make(chan struct{}, 1)
	// Always force at least once run
	forceEventC <- struct{}{}

	batchedEvents := make(chan struct{}, 1)
	go batchExecEvents(w, batchedEvents, ignores, ignoreEvents)

	for {
		if term, err := handleExecEventOnce(w, forceEventC, splitExecStr, ignores, ignoreEvents, batchedEvents); err != nil {
			errC <- err
			return
		} else if term {
			errC <- nil
			return
		}
	}
}

func handleExecEventOnce(w *fsnotify.Watcher, forceEventC chan struct{}, splitExecStr, ignores, ignoreEvents []string, batchedEvents chan struct{}) (bool, error) {
	select {
	case <-forceEventC:
		return runExecCmd(splitExecStr)
	case <-batchedEvents:
		return runExecCmd(splitExecStr)
	case err := <-w.Errors:
		return false, err
	}
}

func runExecCmd(splitExecStr []string) (bool, error) {
	cmd := exec.Command(splitExecStr[0], splitExecStr[1:]...)
	outB, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}
	log.Print("cmd: ", string(outB))
	return false, nil
}

func batchExecEvents(w *fsnotify.Watcher, batchedEvents chan struct{}, ignores, ignoreEvents []string) {
	ignoreMap := make(map[string]struct{})
	for _, s := range ignores {
		ignoreMap[s] = struct{}{}
	}
	ignoreEventMap := make(map[string]struct{})
	for _, s := range ignoreEvents {
		ignoreEventMap[s] = struct{}{}
	}

	ticker := time.NewTicker(time.Second * 5)

	// XXX printBuf should be its own type, so the operations can be struct
	// functions.
	printBuf := make(map[string]int)
	for {
		select {
		case event := <-w.Events:
			if _, ok := ignoreMap[event.Name]; ok {
				msg := fmt.Sprintf("ignore event name: %s", event)
				if _, ok := printBuf[msg]; !ok {
					printBuf[msg] = 0
				}
				printBuf[msg] += 1
				continue
			}
			validEvents := false
			for _, e := range strings.Split(event.Op.String(), "|") {
				if _, ok := ignoreEventMap[e]; !ok {
					validEvents = true
				}
			}
			if !validEvents {
				msg := fmt.Sprintf("ignore event op: %s", event)
				if _, ok := printBuf[msg]; !ok {
					printBuf[msg] = 0
				}
				printBuf[msg] += 1
				continue
			}
			msg := fmt.Sprintf("accept event: %s", event)
			if _, ok := printBuf[msg]; !ok {
				printBuf[msg] = 0
			}
			printBuf[msg] += 1

			select {
			case batchedEvents <- struct{}{}:
				msg := "write flushed batched messages:"
				for k, v := range printBuf {
					msg += fmt.Sprintf("\n[%d] %s", v, k)
				}
				log.Print(msg)
				printBuf = make(map[string]int)
			default:
				// noop for nonblocking
			}

		case <-ticker.C:
			// XXX This ticker is a hack, instead we should have some shared
			// state that is protected by a mutex. Then when the reader
			// side consumes the batched event it can read the printBuf and
			// flush it. The way it is now, the "write flushed" messages
			// can fall behind and not get flushed until the next event.

			// XXX Actually, you'll always need some form of polling flush
			// because otherwise ignore events will never get flushed.

			if len(printBuf) == 0 {
				continue
			}

			msg := "ticker flushed batched messages:"
			for k, v := range printBuf {
				msg += fmt.Sprintf("\n[%d] %s", v, k)
			}
			log.Print(msg)
			printBuf = make(map[string]int)
		}
	}
}
