// implement the execvm toolset
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/moby/sys/mount"
	"golang.org/x/sys/unix"
	"io/ioutil"
	"log"
	"os"
	"syscall"
)

type chainParser struct {
	delimeter string
}

type mountPoint struct {
	Device string
	Target string
	MType string
	Options string
}

type initCommand struct {
	Mounts []mountPoint
	Args []string
}

func listDir(dir string) {
    files, err := ioutil.ReadDir(dir)
    if err != nil {
        log.Fatal(err)
    }

    for _, f := range files {
            fmt.Println(f.Name())
    }
}

func wExitStatus(status unix.WaitStatus) int {
	return (int(status) & 0xff00) >> 8
}

func wTermSig(status unix.WaitStatus) int {
	return int(status) & 0x7f
}

func wIfExited(status unix.WaitStatus) bool {
	return wTermSig(status) == 0
}

func waitPid(pid int) unix.WaitStatus {

	var status unix.WaitStatus

	_, err := unix.Wait4(pid, &status, 0, nil)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return status

}

func exec(environ []string, args []string) {

	err := unix.Exec(args[0], args, environ)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}

func forkExec(environ []string, args []string) int {

	attr := syscall.ProcAttr{
		Dir: "",
		Env: environ,
		Files: []uintptr{
			os.Stdin.Fd(),
			os.Stdout.Fd(),
			os.Stderr.Fd(),
		},
		Sys: nil,
	}

	pid, err := syscall.ForkExec(args[0], args, &attr)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return pid

}

func decodeInitCommand(payload string) []byte {

	data, err := base64.URLEncoding.DecodeString(payload)

	if err != nil {
		fmt.Println("failed to b64decode payload:", err)
		os.Exit(1)
	}

	return data

}

func parseInitCommand(doc []byte) initCommand {

	var cmd initCommand
	err := json.Unmarshal(doc, &cmd)

	if err != nil {
		fmt.Println("failed to parse json:", err)
		os.Exit(1)
	}

	return cmd

}

func findPivot(delimeter string, args []string) (int, error) {

	for i, v := range args {
		if v == delimeter {
			return i, nil
		}
	}

	return -1, errors.New(fmt.Sprintf("Could not find %v in %v", delimeter, args))

}

func (parser chainParser) parse(args []string) ([]string, []string) {

	pivot, err := findPivot(parser.delimeter, args)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return args[:pivot], args[pivot+1:]

}

func initMain(args []string) {

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing payload")
		os.Exit(1)
	}

	doc := decodeInitCommand(args[0])
	cmd := parseInitCommand(doc)

	for _, mnt := range cmd.Mounts {

		err := mount.Mount(mnt.Device, mnt.Target,
			mnt.MType, mnt.Options)

		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

	}

	exec(os.Environ(), cmd.Args)

}

func chainMain(args []string) {

	if len(args) < 4 {
		fmt.Fprintln(os.Stderr, "usage")
		os.Exit(1)
	}

	parser := chainParser{args[0]}
	args1, args2 := parser.parse(args[1:])

	environ := os.Environ()

	pid := forkExec(environ, args1)
	status := waitPid(pid)

	if wIfExited(status) {
		istatus := wExitStatus(status)

		if istatus != 0 {
			os.Exit(istatus)
		}
	} else {
		fmt.Fprintln(os.Stderr, "process failed unexpectedly", args1)
		os.Exit(1)
	}

	exec(environ, args2)

}

func main() {

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		initMain(os.Args[2:])
	case "chain":
		chainMain(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "invalid command", os.Args[1])
	}

	os.Exit(1)

}
