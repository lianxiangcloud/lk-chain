#!/bin/bash
WORKSPACE=$(cd $(dirname $0)/; cd ..; pwd)
PATH=$WORKSPACE/bin/:$PATH
dataDir=$WORKSPACE/data
statedb="state.db"
grep -qe 8192 /proc/sys/fs/inotify/max_user_instances || echo 8192 >/proc/sys/fs/inotify/max_user_instances
grep -qe 10485760 /proc/sys/fs/file-max || echo 10485760 >/proc/sys/fs/file-max
grep -qe 10485760 /proc/sys/fs/nr_open  || echo 10485760 >/proc/sys/fs/nr_open
grep -qe 327680 /proc/sys/kernel/pid_max || echo 327680 > /proc/sys/kernel/pid_max
grep -qe 30000 /proc/sys/net/ipv4/tcp_max_tw_buckets || echo 30000 > /proc/sys/net/ipv4/tcp_max_tw_buckets
ulimit -n 65536
ulimit -u 65536
ulimit -m 1048576 

action=$1

function Init(){

    if [ $# != 2 ]; then
        echo "`Usage`"
        exit 1;
    fi

    logpath=$2/peer_logs/
    dirpath=$2/peer_data/

    if [ ! -d "$dataDir/$statedb" ]; then
        if [ ! -f "$dataDir/$statedb.tar.gz" ]; then
            echo "$dataDir/$statedb.tar.gz need"
            exit 1;
        fi
        tar -zxf "$dataDir/$statedb.tar.gz" -C "$dataDir/"
    fi
    chaindataDir="$dirpath/ethermint/chaindata"
    if [ ! -d "$chaindataDir" ]; then
        mkdir -p "$chaindataDir"
    fi
    cp $dataDir/$statedb/* $chaindataDir

    if [ ! -f "$dataDir/genesis.json" ]; then
            echo "$dataDir/genesis.json need"
            exit 1;
    fi
    tendermintDir="$dirpath/tendermint"
    if [ ! -d "$tendermintDir" ]; then
        mkdir -p "$tendermintDir"
    fi
    cp $dataDir/genesis.json $tendermintDir/

    if [ ! -d "$logpath" ]; then
        mkdir -p "$logpath"
    fi

    echo "init ethermint ..."
    ethermint --datadir $dirpath --lkonline --verbosity 5 --log_file $logpath/ethermint.log init -lkstate=$dataDir/$statedb
    
    echo "init tendermint ..."
    tendermint init --home $tendermintDir --chain_id chain --genesis genesis.json
}

function Start(){
    if [ $# != 2 ]; then
        echo "`Usage`"
        exit 1;
    fi
    
    logpath=$2/peer_logs
    dirpath=$2/peer_data

    nodename="peer"_"${HOSTNAME}"
    ethrpcport=44000
    ethabciport=43000
    tendrpcport=41000
    tendp2pport=42000
    ethsubport=45000
    peerseeds="" #peers to connect
    
    emptyBlockInterval=300
    blockInterval=1000
    blockWarnInterval=350

    echo "start ethermint ..."
    
    nohup ethermint --datadir $dirpath --rpc --rpcaddr=0.0.0.0 --rpcapi eth,net,web3,personal,admin --abci_laddr "tcp://127.0.0.1:$ethabciport" --tendermint_addr "tcp://127.0.0.1:$tendrpcport" --rpcport $ethrpcport --lkrpc_addr "0.0.0.0:$ethsubport" --no_block_create_interval $blockWarnInterval --verbosity 5 --log_file $logpath/ethermint.log node &>>$logpath/ether.log &
    
    sleep 1
    echo "start tendermint ..."
    nohup tendermint --home $dirpath/tendermint --rpc.laddr "tcp://0.0.0.0:$tendrpcport" --proxy_app "tcp://127.0.0.1:$ethabciport" --p2p.laddr "tcp://0.0.0.0:$tendp2pport" --p2p.pex=1 --p2p.seeds "$peerseeds" --eth_rpcaddr "http://127.0.0.1:$ethrpcport" --moniker $nodename --consensus.create_empty_blocks_interval $emptyBlockInterval --consensus.timeout_commit $blockInterval --log_level info --log_file $logpath/tendermint.log node &>>$logpath/tender.log &
    echo "start success!"
}

function Stop(){
    if [ $# != 1 ]; then
        echo "`Usage`"
        exit 1;
    fi

    nodetype=peer
    port=43000

    pid=$(ps -ef | grep $nodetype | grep $port | grep -v grep | awk '{print $2}')

    for i in $pid; do
        echo "kill $i"
        kill -9 $i 2> /dev/null
    done
    echo -e "Stop $nodetype service:\t\t\t\t\t[  OK  ]"
}

function Rollback(){
    if [ $# != 3 ]; then
        echo "`Usage`"
        exit 1;
    fi

    ethermintdirpath=$2/peer_data/
    tendermintdirpath=$2/peer_data/tendermint/
    height=$3

    echo "rollback ethermint ..."

    ethermint rollback --datadir $ethermintdirpath --lkrollbackbyblocknumber $height

    echo "rollback tendermint ..."

    tendermint rollback --home $tendermintdirpath --rollbackblocknumber $height
}

function Usage()
{
    echo ""
    echo "USAGE:"
    echo "command1: $0 init datapath"
    echo ""
    echo "command2: $0 start datapath"
    echo ""
    echo "command3: $0 stop"
    echo ""
    echo "command4: $0 rollback datapath height"
    echo ""
}

case $1 in
    init) Init $@;;
    start) Start $@;;
    stop) Stop $@;;
    rollback) Rollback $@;;
    *) Usage;;
esac
