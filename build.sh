#!/bin/bash

ROOTDIR=`pwd`
BIN=${ROOTDIR}/bin

ethermint=${ROOTDIR}/tendermint/ethermint
tendermint=${ROOTDIR}/tendermint/tendermint

pack=pack
packfile=lk-chain
tarfile=lk-chain-linux-x64.tar.gz
packdst=pack/$packfile

if [ ! -e $BIN ]; then
    mkdir $BIN
fi

function build_ethermint()
{
    echo "building ethermint..."
    cd $ethermint && bash build.sh
    if [ $? -ne 0 ]; then
	    echo "ERROR: build ethermint failed."
	    exit 1
    fi

    cd $ROOTDIR

    cp ${ethermint}/build/_workspace/bin/ethermint $BIN
}

function build_tendermint()
{    
    echo "building tendermint..."
    cd $tendermint && bash build.sh
    if [ $? -ne 0 ]; then
	    echo "ERROR: build tendermint failed."
	    exit 1
    fi

    cd $ROOTDIR
    rm -f ${BIN}/tendermint

    cp ${tendermint}/build/_workspace/bin/tendermint $BIN
}

function build_all()
{
    build_ethermint

    build_tendermint

    echo "build complete."
}

function clean_files()
{
    echo "clean bin..."
    rm -fr ${BIN}
    echo "clean ethermint..."
    rm -fr ${ethermint}/build
    rm -fr ${ethermint}/vendor
    echo "clean tendermint..."
    rm -fr ${tendermint}/build
    rm -fr ${tendermint}/vendor
    echo "clean pack..."
    rm -fr pack
    echo "clean complete"
}

function usage()
{
    echo "Usage:"
    echo "$0 [all|clean|ethermint|tendermint]"
    exit 1
}

function do_pack()
{
    echo "pack"
    cd $ROOTDIR
    rm -rf $pack
    mkdir -p $packdst
    mkdir -p $packdst/bin
    mkdir -p $packdst/data
    mkdir -p $packdst/sbin

    cp bin/ethermint $packdst/bin
    cp bin/tendermint $packdst/bin
    cp tools/start.sh $packdst/sbin
    cp tools/genesis.json $packdst/data
    cp ethermint_init_satate/state.db_20190510.tar.gz $packdst/data/state.db.tar.gz

    cd pack ; tar zcf $tarfile $packfile ; echo "done $tarfile";
}

function main()
{
    case "$1" in
    -h|--help)
        usage
        exit 0
        ;;
    ethermint)
        build_ethermint
        exit 0
        ;;
    tendermint)
        build_tendermint
        exit 0
        ;;
    clean)
        clean_files
        exit 0
        ;;
    *)
        build_all
        ;;
    esac

    do_pack
}

#########################
main "$@"
