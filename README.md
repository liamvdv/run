# run
Are you fed up with typing `./super/long/path/to/script/updateGo.sh`? Do you suffer from not remembering where your script resides? Or even worse, do you often hop between Linux and Windows (or use [wsl](https://docs.microsoft.com/en-us/windows/wsl/about)) and consistent script names and locations are impossible? Then `run` might be a good fit.

`run` is a way to manage and execute scripts across platforms. (macOS, Linux and Windows)

## What does run do?
`run` is a place to store your shell scripts and associate names with them, which `run` calls `cmd` (command). It lets you easily call these commands and remembers, where the script resides. Moreover, `run` detects the platform it is run on, so that you can create different, platform-specific scripts with the same name.
`run` also helps you to keep track of all your scripts (see [`-list` command](#-list)).

## Usage
If you haven't yet initialised the command:
```
$   run -init
```
##### Create a new command:
The `-new` command requires two arguments: the name of the command, the path of the command
```
$   run -new <cmd> <script> [<mininumNumberOfArgs> <maximumNumberOfArgs>]
```
For example `fetchOSINTInformation.sh` takes at least one argument, a username. 
```
$   run -new sherlock ./fetchOSINTInformation.sh 1
```
##### Run a command:
```
$   run sherlock
>>> Wrong argument count passed.
$   run sherlock liamvdv
```

##### Modify a command:
The `-mod` command is comparable to the `-new` command, but requires as an first argument an existing command. If you would like to use the old values, use `_` (underscore).
```
$   run -mod <cmd> <newCmdName> <newScriptPath> [<mininumNumberOfArgs> <maximumNumberOfArgs>]
```
For your `sherlock` example, let's change the name to `sher` and limit the number of arguments passed to our script to 4.
```
$   run -mod sherlock sher _ _ 4
```
##### Delete a command:
The `-del` command is used to delete a user command. You cannot delete internal commands.
```
$   run -del <cmd>
```
##### List all commands:
The `-list` command is used to list all commands, including the internal commands. 
```
$ run -list
>>> run commands:
Name       Location
-init      internal
-new       internal
-mod       internal
-del       internal
-tidy      internal
-list      internal
sher       /home/liamvdv/some/where/fetchOSINTInformation.sh
```
##### Tidy your scripts
The `-tidy` command is the most opaque command semantically, but it is quite simple. `-tidy` moves all scripts to a single folder, which is `~/.run/cmd/:platform/`. The :platform part is either `windows` or `unix`. `unix` was chosen because macOS and Linux distros mostly have the same shell.
We need to run the command with sudo, because -tidy needs access to all folders where you placed your scripts in.
```
$   sudo run -tidy
```
Following our example, `-list` will now show a different location:
```
$   sudo run -tidy
$   run -list
>>> run commands:
Name       Location
-init      internal
-new       internal
-mod       internal
-del       internal
-tidy      internal
-list      internal
sher       /home/liamvdv/.run/cmd/unix/fetchOSINTInformation.sh
```
Quick Note: `-tidy` does not handle scripts with the same name currently. This will be corrected in future commits. 
## Installation
Currently, there is no pre-build version available. You need to have [go@1.16](https://golang.org/doc/go1.16) or higher installed to compile the application. 
#### Linux
First, let's check if go ist installed and if it's above version 1.16. Additionally, we need to know the installation path.
```
$   go version
```
For me, this returns `go version go1.16.3 linux/amd64`, for you it might be different. If the shell tells you that "go" is not known, type `which go` to find the installation location. If none appears, please [download](https://golang.org/dl/) it.
If `which` returns a path, `go` is not yet available on the command prompt. To make `go` available in the prompt we need to add that directory where the `go` executable resides in to the `PATH` enviroment variable. For everyone who installed `go` with the default parameters set, this location should be `/usr/local/go/bin/go`. Drop the last go and type the following (replace `/usr/local/go/bin` if your got a different path).
```
$   echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.bashrc
```
This will add `export PATH=$PATH:/usr/local/go/bin` to your shell configuration, which enables you to access `go`.

Second, let's actually start installing.
1) Download this repository.
2) Open the folder on the terminal and type the following command. `sudo` will ask your for root permissions if you aren't already root.
```
$   sudo ./setup.sh $(which go)
```
3) Initialise the application:
```
$   run -init
```
#### Windows
The installion for Windows is easy if you have go version 1.16 or higher installed. If not, download [download](https://golang.org/dl/) it. Remember that the installation directory (for most people that will be `C:\Program Files\Go\bin`) must be in the PATH environment variable. Check that by typing 
```
$   go version
```
If you don't get a result, ask Google about your specific error message. Tip: Closing and reopening all terminals helps with the most common problem.

Now to the actual installation:
1) Download this repository.
2) Open the folder in the file explorer and **run as administrator** `setup.bat` (not setup.sh!).
3) Initialise the application:
```
$   run -init
```