package main

import (
	_ "embed" // See https://golang.org/pkg/embed/
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// Do not remove. Functional comment. See https://golang.org/pkg/embed/
//go:embed What_is_this.txt
var WHAT_IS_THIS_MSG []byte

func setUp(scriptDp, indexFp string) error {
	if err := os.MkdirAll(scriptDp, 0750); err != nil {
		return err
	}
	switch _, err := os.Stat(indexFp); {
	case err == nil: // nothing, file exists
	case os.IsNotExist(err):
		file, err := os.Create(indexFp)
		if err != nil {
			return err
		}
		defer func() {
			if err := file.Close(); err != nil {
				panic(err) // do not edit, intentionally panics.
			}
		}()
		if _, err := file.Write([]byte{'[', ']'}); err != nil {
			return err
		}
	case err != nil:
		return err
	}

	platformDir := filepath.Dir(scriptDp)
	whatIsThisFp := filepath.Join(platformDir, "What_is_this?")
	switch _, err := os.Stat(whatIsThisFp); {
	case err == nil:
		return err
	case os.IsNotExist(err):
		if err := os.WriteFile(whatIsThisFp, WHAT_IS_THIS_MSG, 0750); err != nil {
			return err
		}
	case err != nil:
		return err
	}

	return nil
}

/******************************************************************************/

var InvalidJsonErrTemplate = "Invalid JSON template: %s \n Please check cmd_mapping.json\n"
var InvalidPathToScriptErr = fmt.Errorf("There is no such script in the provided directory.")
var USAGE_NEW = "Usage:\n\run -new <name> <scriptPath> [<minArgsCount> <maxArgsCount>]"

// createCmd only wants the args that are unspecific to the call of createCmd,
// i. e. $ run -new make make.sh 2 3 will result in [make, make.sh, 2, 3].
// Will by default not set an upper or lower bound for max or min arguments. (i.e. 0 and -1)
func createCmd(indexFp string, args []string) error {
	cmd := jsonCmd{
		Meta: meta{
			MaxNumArgs: -1, // allow any number of args by default
		},
	}
	if err := parseCmd(args, &cmd); err != nil {
		return err
	}

	if _, err := os.Stat(cmd.Script); os.IsNotExist(err) {
		return InvalidPathToScriptErr
	}

	rawJson, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	if err := appendToIndex(indexFp, rawJson); err != nil {
		return err
	}

	return nil
}

/******************************************************************************/

const USAGE_MOD = "Usage:\nrun -mod <cmd> <newName> <newScriptPath> [<minArgsCount> <maxArgsCount>]\n Underscore denote old value."

func modifyCmd(indexFp string, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("Wrong argument count passed.\n%s\n", USAGE_MOD)
	}
	name := args[0]

	modify := func(cmds *[]jsonCmd) error {
		// find command which should be updated
		cmdss := *cmds
		var idx int
		for ; idx < len(cmdss); idx++ {
			if cmdss[idx].Name == name {
				break
			}
		}

		// if no matching command was found, tell the user and do not
		// rewrite the index file. (therefore error)
		if cmdss[idx].Name != name {
			return CmdNotFoundErr
		}

		// allow old values
		switch {
		case args[1] == "_":
			args[1] = cmdss[idx].Name
		case args[2] == "_":
			args[2] = cmdss[idx].Script
		case args[3] == "_":
			args[3] = fmt.Sprintf("%d", cmdss[idx].Meta.MinNumArgs)
		case args[4] == "_":
			args[4] = fmt.Sprintf("%d", cmdss[idx].Meta.MaxNumArgs)
		}

		// make updated command
		cmd := jsonCmd{}
		if err := parseCmd(args[1:], &cmd); err != nil {
			return fmt.Errorf("%w%s\n", err, USAGE_MOD)
		}

		// add it so that updateIndex can rewrite it.
		cmdss[idx] = cmd
		return nil
	}

	return updateIndex(indexFp, modify)
}

/******************************************************************************/

const USAGE_DEL = "Usage:\nrun -del <cmd> [<cmd2> ...]\n"

func deleteCmd(indexFp string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(USAGE_DEL)
	}
	ignore := make(map[string]struct{}, len(args))
	for _, cmd := range args {
		ignore[cmd] = struct{}{}
	}

	//TODO: more efficient to swap and then slice the array? (would also
	// prevent heap allocs I think).
	delete := func(cmds *[]jsonCmd) error {
		cmdss := *cmds
		filtered := make([]jsonCmd, 0, len(cmdss))
		for i := range cmdss {
			if _, yes := ignore[cmdss[i].Name]; yes {
				delete(ignore, cmdss[i].Name) // to build check sum
				continue
			}
			filtered = append(filtered, cmdss[i])
		}
		for k := range ignore {
			fmt.Printf("Could not delete %q, doesn't exist.\n", k)
		}
		*cmds = filtered
		return nil
	}

	return updateIndex(indexFp, delete)
}

/******************************************************************************/

func tidyCmd(scriptDp, indexFp string) error {
	tidy := func(cmds *[]jsonCmd) error {
		cmdss := *cmds
		for i := range cmdss {
			scriptName := filepath.Base(cmdss[i].Script)
			newPath := filepath.Join(scriptDp, scriptName)
			if err := os.Rename(cmdss[i].Script, newPath); err != nil {
				fmt.Printf("Failed to move %q to %q: %s\n", scriptName, newPath, err.Error())
				continue
			}
			cmdss[i].Script = newPath
		}
		return nil
	}

	return updateIndex(indexFp, tidy)
}

/******************************************************************************/

func listCmd(scriptDp, indexFp string) error {
	templt := "%-10s %s\n"
	intTemplt := "%-10s internal\n"

	fmt.Println("run commands:")
	fmt.Printf(templt, "Name", "Location")
	fmt.Printf(intTemplt+intTemplt+intTemplt+intTemplt+intTemplt+intTemplt,
		"-init", "-new", "-mod", "-del", "-tidy", "-list")

	print := func(cmds *[]jsonCmd) error {
		cmdss := *cmds
		for _, cmd := range cmdss {
			fmt.Printf("%-10s %s\n", cmd.Name, cmd.Script)
		}
		return nil // TODO: implement a dummy error to prevent rewriting file?
	}
	return updateIndex(indexFp, print)
}

/******************************************************************************/
// Helpers

func parseCmd(args []string, cmd *jsonCmd) (err error) {
	var i int
	switch l := len(args); true {
	case l >= 2:
		cmd.Name = args[0]
		cmd.Script, err = filepath.Abs(args[1])
		fallthrough
	case l >= 3:
		i, err = strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		cmd.Meta.MinNumArgs = i
		fallthrough
	case l >= 4:
		i, err = strconv.Atoi(args[3])
		if err != nil {
			return err
		}
		cmd.Meta.MaxNumArgs = i
	default:
		return fmt.Errorf("Wrong argument count passed.\n")
	}
	return nil
}

func appendToIndex(indexFp string, rawJson []byte) error {
	file, err := os.OpenFile(indexFp, os.O_RDWR|os.O_CREATE, 0550)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	// if file is empty ("") or ("[]\n"), just add json
	if fi.Size() <= 3 {
		cap := len(rawJson) + 2
		buf := make([]byte, cap)
		buf[0] = '['
		copy(buf[1:cap], rawJson)
		buf[cap-1] = ']'
		if _, err := file.Write(buf); err != nil {
			return err
		}
		return nil
	}
	// Else, allow efficient writes by appending to end.
	// account for trailing spaces:

	// read up to 9 other bytes till ']' is the last.
	// " x x x x x x x x x ] ... "
	// later write a max of 9 other bytes + ',' + rawJson + ']'.
	// " x x x x x x x x x , jsonRaw ] "
	buf := make([]byte, 10+len(rawJson)+1)

	offset := fi.Size() - 10
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	n, err := file.Read(buf)
	if err != nil { // does NOT return io.EOF because we read exactly till end.
		return err
	}

	// find ']', the real end of json
	var end = -1
	for i, b := range buf[:n] { // :n actually not needed?
		if b == ']' {
			end = i
			break
		}
	}
	if end == -1 {
		return fmt.Errorf(InvalidJsonErrTemplate, end)
	}

	// replace ']'
	buf[end] = ','
	// append serialised cmd
	copy(buf[end+1:], rawJson)
	// add ']' again
	cap := end + 2 + len(rawJson)
	buf[cap-1] = ']'
	if _, err := file.WriteAt(buf[:cap], offset); err != nil {
		return err
	}
	return nil
}

// cmd is just a strcut that should be filled.
func findInIndex(indexFp string, name string, cmd *jsonCmd) error {
	file, err := os.Open(indexFp)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	dec := json.NewDecoder(file)
	// read the opening '['
	t, err := dec.Token()
	if err != nil {
		return err // TODO
	}
	if t != json.Delim('[') {
		return fmt.Errorf(InvalidJsonErrTemplate, t) //TODO: actually valid but not expected
	}
	hit := false
	for dec.More() {
		var cur jsonCmd
		if err := dec.Decode(&cur); err != nil {
			return err
		}
		if cur.Name == name {
			*cmd = cur
			hit = true
			break
		}
	}
	if !hit {
		return CmdNotFoundErr
	}
	return nil
}

// updateIndex loads the hole index file into memory and parses it. Then it
// calls fn and exits if an error is returned. Else it will write cmds back into
// the index file.
// TODO: Do not load hole file into memory. Maybe create a shadow file and later
// delete the originial and rename the shadow file.
// Requires change to the api, fn should only take one cmd.
func updateIndex(indexFp string, fn func(cmds *[]jsonCmd) error) error {
	rawJson, err := os.ReadFile(indexFp)
	if err != nil {
		return err
	}

	var cmds []jsonCmd

	if err := json.Unmarshal(rawJson, &cmds); err != nil {
		return err
	}

	if err := fn(&cmds); err != nil {
		return err
	}

	file, err := os.OpenFile(indexFp, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0660)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	rawJson, err = json.Marshal(cmds)
	if err != nil {
		return err
	}

	_, err = file.Write(rawJson)
	return err
}
