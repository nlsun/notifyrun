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

type watch struct {
	execStrs     []string
	ignoreStrs   []string
	ignoreEvStrs []string
	files        []string
}

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

	w, err := newWatch(execStr, ignoreStrSlice, ignoreEventStrSlice, args)
	if err != nil {
		return err
	}

	if execStr != "" {
		return w.execAction()
	}
	return fmt.Errorf("must select an action type")
}

func newWatch(execStr string, ignoreStrSlice, ignoreEventStrSlice, args []string) (*watch, error) {
	splitExecStr, err := shlex.Split(execStr)
	if err != nil {
		return nil, err
	}

	return &watch{
		execStrs:     splitExecStr,
		ignoreStrs:   ignoreStrSlice,
		ignoreEvStrs: ignoreEventStrSlice,
		files:        args,
	}, nil
}

func (w *watch) execAction() error {
	if len(w.files) == 0 {
		return fmt.Errorf("must specify files/directories to watch")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	errC := make(chan error)
	go w.handleExecEvents(watcher, errC)

	log.Print("watching: ", w.files)
	for _, f := range w.files {
		if err := watcher.Add(f); err != nil {
			return err
		}
	}
	return <-errC
}

func (w *watch) handleExecEvents(fsw *fsnotify.Watcher, errC chan error) {
	forceEventC := make(chan struct{}, 1)
	// Always force at least one run
	forceEventC <- struct{}{}

	batchedEvents := make(chan struct{}, 1)
	go w.batchExecEvents(fsw, batchedEvents)

	for {
		if err := w.handleExecEventOnce(fsw, forceEventC, batchedEvents); err != nil {
			errC <- err
			return
		}
	}
}

func (w *watch) handleExecEventOnce(fsw *fsnotify.Watcher, forceEventC chan struct{}, batchedEvents chan struct{}) error {
	select {
	case <-forceEventC:
		return runExecCmd(w.execStrs)
	case <-batchedEvents:
		return runExecCmd(w.execStrs)
	case err := <-fsw.Errors:
		return err
	}
}

func runExecCmd(splitExecStr []string) error {
	cmd := exec.Command(splitExecStr[0], splitExecStr[1:]...)
	outB, err := cmd.CombinedOutput()
	if err == nil {
		log.Print("cmd: ", string(outB))
		return nil
	}

	// If the command fails, log the error and carry on.
	if _, ok := err.(*exec.ExitError); ok {
		log.Print("cmd error out: ", string(outB))
		log.Print("cmd error err: ", err)
		return nil
	}

	return err
}

func (w *watch) batchExecEvents(fsw *fsnotify.Watcher, batchedEvents chan struct{}) {
	ignoreMap := make(map[string]struct{})
	for _, s := range w.ignoreStrs {
		ignoreMap[s] = struct{}{}
	}
	ignoreEventMap := make(map[string]struct{})
	for _, s := range w.ignoreEvStrs {
		ignoreEventMap[s] = struct{}{}
	}

	ticker := time.NewTicker(time.Second * 5)

	// XXX printBuf should be its own type, so the operations can be struct
	// functions.
	printBuf := make(map[string]int)
	for {
		select {
		case event := <-fsw.Events:
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
			// because otherwise ignore events will never get flushed. Or
			// have a separate channel for flushing.

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
