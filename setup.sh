#!/bin/bash
# build source
if [ "$EUID" -ne 0 ]
    then 
        echo "Remember to run this as administrator."
        echo "  $ sudo ./script.sh <path to go installation>"
        exit
fi

# Need to set PATH, because script will not read ~/.bashrc
GOINSTALLPATH=$(dirname $1)
export PATH=$PATH:$GOINSTALLPATH
go build -o run main.go cmd.go
mv ./run /usr/local/bin/run
