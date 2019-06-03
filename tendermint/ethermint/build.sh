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
  tar zxf ../../vendor.tar.gz 
  echo "tar vendor success!"
fi

rm -rf ./vendor/third_part
cd ./vendor
ln -s ../../../third_part .
cd ..

rm -rf ./vendor/github.com/tendermint/tendermint
rm -rf ./vendor/github.com/ethereum
cd ./vendor/github.com
ln -s ../../../../ethereum .
cd ./tendermint
rm -rf ../../../../tendermint/vendor
ln -s ../../../../tendermint .

cd "$root"

tenderdir="$workspace/src/github.com/tendermint/"
if [ ! -L "$tenderdir/ethermint" ]; then
    mkdir -p "$tenderdir"
    cd "$tenderdir"
    ln -s ../../../../../../ethermint .
    cd "$root"
fi

# Run the command inside the workspace.
cd "$tenderdir/ethermint"
PWD="$tenderdir/ethermint"
echo "start build ethermint ...."
make install
echo "build ethermint success!"
