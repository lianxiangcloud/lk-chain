#!/bin/sh

set -e

# Create workspace if it doesn't exist yet.
root="$PWD"
workspace="$PWD/build/_workspace"
if [ ! -L "$workspace" ]; then
	mkdir -p "$workspace"
fi

# Set up the environment to use the workspace.
export GOPATH="$workspace"

# unzip vendor
if [ ! -d "vendor" ]; then
    echo "start tar vendor package ...."
    tar -zxf ../../vendor.tar.gz
    echo "tar vendor success!"
fi

if [ ! -L "vendor/third_part" ]; then
    echo "ln third_part"
    ln -s ../../../third_part vendor/third_part
fi

if [ ! -L "vendor/github.com/ethereum/go-ethereum" ]; then
    echo "ln github.com/ethereum"
    ln -s ../../../../ethereum vendor/github.com/ethereum
fi

tenderdir="$workspace/src/github.com/tendermint/"
if [ ! -L "$tenderdir/tendermint" ]; then
    mkdir -p "$tenderdir"
    cd "$tenderdir"
    ln -s ../../../../../../tendermint tendermint
    cd "$root"
fi

# Run the command inside the workspace.
cd "$tenderdir/tendermint"
PWD="$tenderdir/tendermint"
echo "start build tendermint ...."
make install
echo "build tendermint success!"
