# LK-Chain

[![License](https://img.shields.io/badge/license-GPLv3.0%2B-blue.svg)](https://www.gnu.org/licenses/gpl-3.0.html)

Supporting golang implementation of the Ethereum protocol.

## Building the source

Building lk-chain requires Go (version 1.9.2).  
System Centos 7.
To build the full suite of utilities:

```shell
./build.sh all
```

or, to clean build path:

```shell
./build.sh clean
```

## Executables

The LK-Chain project comes with several wrappers/executables found in the `pack/lk-chain/bin` directory.

| Command         | Description |
|:---------------:|-------------|
| **`ethermint`** | Ethereum protocol |
| **`tendermint`** | Tendermint Core is Byzantine Fault Tolerant (BFT) middleware |

## Running LK-Chain

Going through all the possible command line flags is out of scope here (please consult our
[CLI Wiki page](https://github.com/ethereum/go-ethereum/wiki/Command-Line-Options)), but we've
enumerated a few common parameter combos to get you up to speed quickly on how you can run your
own Ethermint and Tendermint instance.

### Full node on the main LK-Chain network

```shell
# go to sbin path.
cd pack/lk-chain/sbin

# init the node data in the first time.
./start.sh init DataPath

# start the node
./start.sh start DataPath

# if you want to stop the instance
./start.sh stop
```

### Configuration

ethermint Configuration:

- **--rpcport** 44000
- **--lkrpc_addr** 0.0.0.0:45000

tendermint Configuration:

- **--rpc.laddr** tcp://0.0.0.0:41000

### Programatically interfacing LK-Chain nodes

The IPC interface is enabled by default and exposes all the APIs supported by Ethermint, whereas the HTTP
and WS interfaces need to manually be enabled and only expose a subset of APIs due to security reasons.
These can be turned on/off and configured as you'd expect.

You can use ethermint like geth console:

```shell
[root@localhost sbin]# ../bin/ethermint attach http://127.0.0.1:44000
Welcome to the Geth JavaScript console!

 modules: admin:1.0 eth:1.0 net:1.0 personal:1.0 rpc:1.0 web3:1.0

> eth.blockNumber
65719
```

or request a JSON-RPC API,like this:

```shell
// Request
curl -X POST --data '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xc94770007dda54cF92009BFF0dE90c06F603a09f", "latest"],"id":1}' -H 'Content-Type:application/json' http://127.0.0.1:44000

// Result
{
  "id":1,
  "jsonrpc": "2.0",
  "result": "0x0234c8a3397aab58" // 158972490234375000
}
```

Ethermint HTTP based JSON-RPC API options:

- `--rpc` Enable the HTTP-RPC server
- `--rpcaddr` HTTP-RPC server listening interface (default: "localhost")
- `--rpcport` HTTP-RPC server listening port (default: 8545)
- `--rpcapi` API's offered over the HTTP-RPC interface (default: "eth,net,web3")
- `--rpccorsdomain` Comma separated list of domains from which to accept cross origin requests (browser enforced)
- `--ws` Enable the WS-RPC server
- `--wsaddr` WS-RPC server listening interface (default: "localhost")
- `--wsport` WS-RPC server listening port (default: 8546)
- `--wsapi` API's offered over the WS-RPC interface (default: "eth,net,web3")
- `--wsorigins` Origins from which to accept websockets requests
- `--ipcdisable` Disable the IPC-RPC server

Find more JSON-RPC APIs on file **doc/json-rpc.md**.
