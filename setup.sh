#!/bin/bash
# build src
export PATH=$PATH:/usr/local/go/bin
echo "Remember to run this as administrator."
echo "  $ sudo ./script.sh"
go build -o run main.go cmd.go
mv ./run /usr/local/bin/run
