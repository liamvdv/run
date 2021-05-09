package main

import (
	"bufio"
	_ "embed" // See https://golang.org/pkg/embed/
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Do not remove. Functional comment. See https://golang.org/pkg/embed/
//go:embed What_is_this.txt
var WHAT_IS_THIS_MSG []byte

func SetUp(scriptDp, indexFp string) error {
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

	runDir := filepath.Dir(filepath.Dir(scriptDp))
	whatIsThisFp := filepath.Join(runDir, "What_is_this.txt")
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
var USAGE_NEW = "Usage:\n\trun -new <name> <scriptPath> [<minArgsCount> <maxArgsCount>]"

// CreateCmd only wants the args that are unspecific to the call of CreateCmd,
// i. e. $ run -new make make.sh 2 3 will result in [make, make.sh, 2, 3].
// Will by default not set an upper or lower bound for max or min arguments. (i.e. 0 and -1)
func CreateCmd(indexFp string, args []string) error {
	cmd := jsonCmd{
		Meta: meta{
			MaxNumArgs: -1, // allow any number of args by default
		},
	}
	if err := parseCmd(args, &cmd); err != nil {
		return fmt.Errorf("%w%s", err, USAGE_NEW)
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

const USAGE_MOD = "Usage:\n\trun -mod <cmd> <newName> [<newScriptPath> [<minArgsCount> <maxArgsCount>]]\n\nAn underscore (_) denotes the orginal value."

func ModifyCmd(indexFp string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("Wrong argument count passed.\n%s\n", USAGE_MOD)
	}
	name, updateArg := args[0], args[1:]
	var hit bool

	// Will still result in rewriting hole index file, because we cannot know
	// if the file was changed, thus cannot set esc.
	var modify modFn = func(cmd *jsonCmd) (inc bool, esc bool, err error) {
		inc = true

		if cmd.Name != name {
			return
		}
		hit = true

		// allow old values
		n := cmd.Name
		s := cmd.Script
		min := fmt.Sprintf("%d", cmd.Meta.MinNumArgs)
		max := fmt.Sprintf("%d", cmd.Meta.MaxNumArgs)

		l := len(updateArg)

		if updateArg[0] != "_" {
			n = updateArg[0]
		}
		if l >= 2 && updateArg[1] != "_" {
			s = updateArg[1]
		}
		if l >= 3 && updateArg[2] != "_" {
			min = updateArg[2]
		}
		if l >= 4 && updateArg[3] != "_" {
			max = updateArg[3]
		}

		// make updated command
		if err := parseCmd([]string{n, s, min, max}, cmd); err != nil {
			return inc, esc, fmt.Errorf("%w%s\n", err, USAGE_MOD)
		}
		return
	}

	if err := modOperation(indexFp, modify); err != nil {
		return err
	}

	if !hit {
		return CmdNotFoundErr
	}

	return nil
}

/******************************************************************************/

const USAGE_DEL = "Usage:\n\trun -del <cmd> [<cmd2> ...]\n"

func DeleteCmd(indexFp string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(USAGE_DEL)
	}

	excl := make(map[string]struct{}, len(args))
	for _, cmd := range args {
		excl[cmd] = struct{}{}
	}

	rm := make(map[string]struct{}, len(args))

	var incl modFn = func(cmd *jsonCmd) (inc, esc bool, err error) {
		// inc = false; esc = false; err = nil
		if _, yes := excl[cmd.Name]; yes {
			rm[cmd.Name] = struct{}{}
			return
		}
		inc = true
		return
	}

	if err := modOperation(indexFp, incl); err != nil {
		return err
	}
	// if not all unique commands have been found
	if len(rm) < len(excl) {
		for k := range excl {
			if _, yes := rm[k]; !yes {
				fmt.Printf("Cannot delete non-existent command %q.\n", k)
			}
		}
		fmt.Println("See all commands:\n\trun -list")
	}
	return nil
}

/******************************************************************************/

func TidyCmd(scriptDp, indexFp string) error {
	entries, err := os.ReadDir(scriptDp)
	if err != nil {
		return err
	}
	takenNames := make(map[string]struct{}, len(entries)+20)
	for _, entry := range entries {
		takenNames[entry.Name()] = struct{}{}
	}

	// tidy moves all scripts into a single directory. This has two
	// effects:
	// 1) Namespacing through abspath doesn't work anymore, we have to
	//    activly prevent name collisions.
	// 2) The IO should be reduced, i. e. the calls to os.Rename should
	//    be limited. To do so check if script is already in the dir.
	var tidy modFn = func(cmd *jsonCmd) (inc, esc bool, err error) {
		inc = true

		scriptName := filepath.Base(cmd.Script)

		// check if already in registry
		if strings.HasPrefix(cmd.Script, scriptDp) {
			return
		}
		// check for name collison
		if _, exists := takenNames[scriptName]; exists {
			// search for fitting name. Pattern: name + NUM_ASC + ext; start 1
			// f. e. update.sh -> update1.sh
			ext := filepath.Ext(scriptName)
			n := 1
			name := cmd.Script[:len(cmd.Script)-len(ext)]
			pattern := name + "%d" + ext

			var newName = fmt.Sprintf(pattern, n)
			for {
				if _, exist := takenNames[newName]; exist {
					n++
					newName = fmt.Sprintf(pattern, n)
					continue
				}
				break
			}
			fmt.Printf("Renaming %s to %s because of script name collision in registry.", scriptName, newName)
			scriptName = newName
		}

		newPath := filepath.Join(scriptDp, scriptName)
		if err := os.Rename(cmd.Script, newPath); err != nil {
			fmt.Printf("Failed to move %q to %q: %s\n", scriptName, newPath, err.Error())
			return inc, esc, err
		}
		cmd.Script = newPath
		return
	}

	return modOperation(indexFp, tidy)
}

/******************************************************************************/

func ListCmd(scriptDp, indexFp string) error {
	templt := "%-10s %s\n"
	intTemplt := "%-10s internal\n"

	fmt.Println("run commands:")
	fmt.Printf(templt, "Name", "Location")
	for _, cmd := range InternalCmds {
		fmt.Printf(intTemplt, cmd)
	}

	var print findFn = func(cmd *jsonCmd) (esc bool, err error) {
		fmt.Printf("%-10s %s\n", cmd.Name, cmd.Script)
		return
	}
	return findOperation(indexFp, print)
}

/******************************************************************************/
// Helpers

func parseCmd(args []string, cmd *jsonCmd) (err error) {
	var i int
	var ran = false
	l := len(args)
	if l >= 2 {
		ran = true
		cmd.Name = args[0]
		cmd.Script, err = filepath.Abs(args[1])
	}
	if l >= 3 {
		i, err = strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		cmd.Meta.MinNumArgs = i
	}
	if l >= 4 {
		i, err = strconv.Atoi(args[3])
		if err != nil {
			return err
		}
		cmd.Meta.MaxNumArgs = i
	}
	if !ran {
		return fmt.Errorf("Wrong argument count.\n")
	}
	return nil
}

// Easy if reading everything into memory, but that might not be possible for
// large cmd files. Instead, we need to do memory low operations.

// Find return CmdNotFoundErr if no matching command could be found.
func Find(indexFp string, name string, lCmd *jsonCmd) error {
	var hit bool

	var find findFn = func(cmd *jsonCmd) (esc bool, err error) {
		if cmd.Name == name {
			lCmd = cmd
			hit = true
			esc = true
			return
		}
		return
	}
	if err := findOperation(indexFp, find); err != nil {
		return err
	}

	if !hit {
		return CmdNotFoundErr
	}
	return nil
}

// fn func(cmd *jsonCmd) (inc bool, esc bool, err error)
// inc: include the cmd. inc == false will not include the command.
// esc: escape the same as err but semantically more expressive.
// err: immidiately stops all execution and prior changes will not be applied.
type findFn func(cmd *jsonCmd) (esc bool, err error)

func findOperation(indexFp string, fn findFn) error {
	file, err := os.Open(indexFp)
	if err != nil {
		return err
	}
	defer saveClose(file)
	dec := json.NewDecoder(file)

	t, err := dec.Token()
	if err != nil {
		return err
	}
	if t != json.Delim('[') {
		return fmt.Errorf(InvalidJsonErrTemplate, t)
	}

	for dec.More() {
		var cmd jsonCmd
		if err := dec.Decode(&cmd); err != nil {
			return err
		}

		esc, err := fn(&cmd)
		if err != nil {
			return err
		}

		if esc {
			return nil
		}
	}
	return nil
}

// modFn is a callback provided to modOperation, which will be called for every
// command in the index file. The behavior of modOperation can be controlled
// with the return values of modFn.
// inc: include the cmd. inc == false will not include the command.
// esc: immidiately stops all execution and prior changes will not be applied.
// err: same as esc, but will also return error to caller.
type modFn func(cmd *jsonCmd) (inc, esc bool, err error)

func modOperation(indexFp string, fn modFn) error {
	src, err := os.Open(indexFp)
	if err != nil {
		return err
	}
	defer saveClose(src)
	dec := json.NewDecoder(src)

	fpExt := indexFp + ".tmp"
	dst, err := os.OpenFile(fpExt, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0660)
	if err != nil {
		return err
	}

	var rmTmp = true
	defer func(fp string) {
		if rmTmp { // closure because we need value of rmTmp at end of exec.
			if err := os.Remove(fp); err != nil {
				panic(err)
			}
		}
	}(fpExt)

	defer saveClose(dst)
	dstWr := bufio.NewWriter(dst)

	// read '['
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if t != json.Delim('[') {
		return fmt.Errorf(InvalidJsonErrTemplate, t)
	}
	if err := dstWr.WriteByte('['); err != nil {
		return err
	}

	var (
		inc bool
		esc bool
	)
	// another is used to check if we need to insert a ',' before adding rawJson
	another := false
	for dec.More() {
		var cmd jsonCmd
		if err := dec.Decode(&cmd); err != nil {
			return err
		}

		inc, esc, err = fn(&cmd)
		if err != nil {
			return err
		}

		// defered functions will take care, f. e. rm tmp file
		if esc {
			return nil
		}

		if inc {
			raw, err := json.Marshal(cmd)
			if err != nil {
				return err
			}
			if another {
				if err := dstWr.WriteByte(','); err != nil {
					return err
				}
			}
			if _, err := dstWr.Write(raw); err != nil {
				return err
			}
			if !another {
				another = true
			}
		}
	}

	if err := dstWr.WriteByte(']'); err != nil {
		return err
	}

	if err := dstWr.Flush(); err != nil {
		return err
	}

	if err := os.Rename(fpExt, indexFp); err != nil {
		return err
	}
	// defered os.Remove() function unnecessary.
	rmTmp = false
	return nil
}

func appendToIndex(indexFp string, rawJson []byte) error {
	file, err := os.OpenFile(indexFp, os.O_RDWR|os.O_CREATE, 0550)
	if err != nil {
		return err
	}
	defer saveClose(file)

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

func invalidArgsError(cmd *jsonCmd, argsLen int) error {
	var s = "at least"
	var n = cmd.Meta.MinNumArgs
	var plural = "s"
	if argsLen > cmd.Meta.MaxNumArgs {
		s = "at most"
		n = cmd.Meta.MaxNumArgs
	}
	if n == 1 || n == -1 {
		plural = ""
	}

	return fmt.Errorf("%q expects %s %d argument%s.", cmd.Name, s, n, plural)
}

func saveClose(f *os.File) {
	if err := f.Close(); err != nil {
		panic(err)
	}
}
