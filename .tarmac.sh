#!/bin/sh

set -ev

# we always run in a fresh dir in tarmac
export GOPATH=$(mktemp -d)
trap 'rm -rf "$GOPATH"' EXIT

# this is a hack, but not sure tarmac is golang friendly
mkdir -p $GOPATH/src/github.com/ubports/goget-ubuntu-touch
cp -a . $GOPATH/src/github.com/ubports/goget-ubuntu-touch
cd $GOPATH/src/github.com/ubports/goget-ubuntu-touch

./run-checks
