package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	BASE_DIR   string = ".run"
	SCRIPT_DIR string = "cmd"
	INDEX_FILE string = "cmd_mappings.json"
)

var InternalCmds = []string{
	"-init",
	"-new",
	"-mod",
	"-del",
	"-tidy",
	"-list",
}

func main() {
	home, err := userHomeDir() // custom implementation to account for "sudo" command.
	if err != nil {
		GracefulExit(err)
	}
	platform, err := getPlatform()
	if err != nil {
		GracefulExit(err)
	}
	scriptDp := filepath.Join(home, BASE_DIR, SCRIPT_DIR, platform.String()) // ~/.run/cmd/:platform
	indexFp := filepath.Join(scriptDp, INDEX_FILE)                           // ~/.run/cmd/:platform/cmd_mapping.json

	if err := Run(os.Args[1:], scriptDp, indexFp); err != nil {
		GracefulExit(err)
	}
}

// Run expectes all text tokens passed to run, i. e.
// $ run -new cool ./cool.sh => [-new, cool, ./cool.sh]
func Run(runArgs []string, scriptDp, indexFp string) (err error) {
	if len(runArgs) < 1 {
		GracefulExit(USAGE_MSG)
	}

	// check for internal commands
	switch runArgs[0] {
	case "-init":
		return setUp(scriptDp, indexFp)
	case "-new":
		return createCmd(indexFp, runArgs[1:])
	case "-mod":
		return modifyCmd(indexFp, runArgs[1:])
	case "-del":
		return deleteCmd(indexFp, runArgs[1:])
	case "-tidy":
		return tidyCmd(scriptDp, indexFp)
	case "-list":
		return listCmd(scriptDp, indexFp)
	}

	// check for external commands
	// cmd should either be in cmd_mapping.json or if no result is found, it
	// should be a name of a script in the platform folder (without ending).
	// If none of this applies, tell the user that.
	cmd, err := getCommand(scriptDp, runArgs, indexFp)
	if err != nil {
		GracefulExit(err)
	}

	exe := exec.Command(cmd[0], cmd[1:]...)
	exe.Stderr = os.Stderr
	exe.Stdout = os.Stdout
	exe.Stdin = os.Stdin

	err = exe.Run()
	if err != nil && strings.HasSuffix(err.Error(), "exec format error") {
		return fmt.Errorf(MissingShebangErrorMsg)
	}
	return err
}

// GracefulExit does not honor deferred functions.
func GracefulExit(v interface{}) {
	switch val := v.(type) {
	case error:
		fmt.Println(val.Error(), USAGE_MSG)
	default:
		fmt.Println(val)
	}
	os.Exit(0)
}

var USAGE_MSG = `
Usage: 
	run <script_name> [args]
`

/******************************************************************************/

type osType int

const (
	// do not reorder
	UNSUPPORTED osType = iota
	UNIX
	WINDOWS
)

var osTypeToString = []string{
	// do  not reorder
	UNSUPPORTED: "",
	UNIX:        "unix",
	WINDOWS:     "windows",
}

func (t osType) String() string {
	return osTypeToString[t]
}

func getPlatform() (osType, error) {
	switch runtime.GOOS {
	case "linux", "darwin":
		return UNIX, nil
	case "windows":
		return WINDOWS, nil
	default:
		return UNSUPPORTED, fmt.Errorf("run does not support %q as a platform. See github.com/liamvdv/do", runtime.GOOS)
	}
}

/******************************************************************************/

type meta struct {
	MinNumArgs int `json:"minNumArgs"`
	MaxNumArgs int `json:"maxNumArgs"`
}

type jsonCmd struct {
	Name   string `json:"commandName"`
	Script string `json:"scriptName"`
	Meta   meta   `json:"options"`
}

/******************************************************************************/

const MissingShebangErrorMsg = `You need to add a shebang to your script.
A shebang is the first line of your script, for example:
  #!/bin/sh
or
  #!/bin/bash`

var CmdNotFoundErr = fmt.Errorf("Command not found.")

// args is expected to contain all arguments excluding the "run"
func getCommand(dirpath string, args []string, indexFp string) ([]string, error) {
	name := args[0]
	argsToScriptN := len(args) - 1

	cmd := jsonCmd{}
	err := findInIndex(indexFp, name, &cmd)
	if err == nil {
		checks := cmd.Meta
		// -1 allows any number or args
		if !(checks.MinNumArgs <= argsToScriptN) || (checks.MaxNumArgs != -1 && !(argsToScriptN <= checks.MaxNumArgs)) {
			return nil, invalidArgsError(&cmd, argsToScriptN)
		}
		args[0] = cmd.Script
		return args, nil
	}

	if err != nil && !errors.Is(err, CmdNotFoundErr) {
		return nil, err
	}
	defer fmt.Printf("Have you forgot to add your new script to %q?\n", dirpath)

	// no matching command was found. Try helping user by assuming "run MyDing someArg123" == ./MyDing.sh someArg123
	entries, err := os.ReadDir(dirpath)
	if err != nil {
		return nil, err
	}

	containsDir := false
	for _, entry := range entries {
		if entry.IsDir() {
			containsDir = true
			continue
		}
		fName := entry.Name()
		ext := filepath.Ext(fName)
		if fName[:len(fName)-len(ext)] == name {
			args[0] = filepath.Join(dirpath, fName)
			return args, nil
		}
	}
	if containsDir {
		fmt.Printf("You should not have folders in %q. It is only ment for script files.", dirpath)
	}

	return nil, CmdNotFoundErr
}

/******************************************************************************/

// userHomeDir is essentially a copy of os.UserHomeDir, but it detects the user
// who ran the script, not the one executing it. This is important, because
// -tidy requires priviledges. Using sudo will result in $HOME equaling /root.
// Thus, we need to check if sudo is used and act accordingly.
func userHomeDir() (string, error) {
	env, enverr := "HOME", "$HOME"
	switch runtime.GOOS {
	case "windows":
		env, enverr = "USERPROFILE", "%userprofile%"
	case "plan9":
		env, enverr = "home", "$home"
	// inserted case
	case "linux", "darwin":
		// if running as root
		if os.Geteuid() == 0 {
			// check if run with sudo
			if usrname := os.Getenv("SUDO_USER"); usrname != "" {
				if usr, err := user.Lookup(usrname); err == nil {
					return usr.HomeDir, nil
				} else {
					return "", err
				}
			}
		}
	}

	if v := os.Getenv(env); v != "" {
		return v, nil
	}
	// On some geese the home directory is not always defined.
	switch runtime.GOOS {
	case "android":
		return "/sdcard", nil
	case "ios":
		return "/", nil
	}
	return "", errors.New(enverr + " is not defined")
}
