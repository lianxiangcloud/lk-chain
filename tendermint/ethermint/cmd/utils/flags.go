package utils

import (
	"gopkg.in/urfave/cli.v1"
)

var (
	LKRotateLogFileFlag = cli.StringFlag{
		Name:  "log_file",
		Value: "ethermint.log",
		Usage: "rotate logfile",
	}

	LKRoleFlag = cli.StringFlag{
		Name:  "role",
		Value: "validator",
		Usage: "role: validator or peer, default is validator",
	}

	LKNoBlockWarnIntervalFlag = cli.IntFlag{
		Name:  "no_block_warn_interval",
		Value: 300,
		Usage: "no block warn interval seconds, default is 300",
	}

	LKNoBlockCreateIntervalFlag = cli.IntFlag{
		Name:  "no_block_create_interval",
		Value: 5,
		Usage: "no block create interval seconds, default is 5",
	}

	LKRPCAddrFlag = cli.StringFlag{
		Name:  "lkrpc_addr",
		Value: "localhost:15001",
		Usage: "rpc subscribe service",
	}

	LKOnlineFlag = cli.BoolFlag{
		Name:  "lkonline",
		Usage: "online mode",
	}

	LKRollbackBlockByNumberFlag = cli.Int64Flag{
		Name:  "lkrollbackbyblocknumber",
		Usage: "roll back by block number",
		Value: 0,
	}

	LKRateTooManyTxsFlag = cli.Float64Flag{
		Name:  "ratetoomanytxs",
		Usage: "rate of too many txs in txpool or mempool",
		Value: 0.1,
	}

	LKStateFlag = cli.StringFlag{
		Name:  "lkstate",
		Value: "",
		Usage: "init state leveldb dir",
	}

	// ----------------------------
	// ABCI Flags

	// TendermintAddrFlag is the address that ethermint will use to connect to the tendermint core node
	// #stable - 0.4.0
	TendermintAddrFlag = cli.StringFlag{
		Name:  "tendermint_addr",
		Value: "tcp://localhost:46657",
		Usage: "This is the address that ethermint will use to connect to the tendermint core node. Please provide a port.",
	}

	// ABCIAddrFlag is the address that ethermint will use to listen to incoming ABCI connections
	// #stable - 0.4.0
	ABCIAddrFlag = cli.StringFlag{
		Name:  "abci_laddr",
		Value: "tcp://0.0.0.0:46658",
		Usage: "This is the address that the ABCI server will use to listen to incoming connection from tendermint core.",
	}

	// ABCIProtocolFlag defines whether GRPC or SOCKET should be used for the ABCI connections
	// #stable - 0.4.0
	ABCIProtocolFlag = cli.StringFlag{
		Name:  "abci_protocol",
		Value: "socket",
		Usage: "socket | grpc",
	}

	// VerbosityFlag defines the verbosity of the logging
	// #unstable
	VerbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Value: 3,
		Usage: "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=core, 5=debug, 6=detail",
	}

	// ConfigFileFlag defines the path to a TOML config for go-ethereum
	// #unstable
	ConfigFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}

	// TargetGasLimitFlag defines gas limit of the Genesis block
	// #unstable
	TargetGasLimitFlag = cli.Uint64Flag{
		Name:  "targetgaslimit",
		Usage: "Target gas limit sets the artificial target gas floor for the blocks to mine",
		Value: GenesisTargetGasLimit.Uint64(),
	}

	// WithTendermintFlag asks to start Tendermint
	// `tendermint init` and `tendermint node` when `ethermint init`
	// and `ethermint` are invoked respectively.
	WithTendermintFlag = cli.BoolFlag{
		Name: "with-tendermint",
		Usage: "If set, it will invoke `tendermint init` and `tendermint node` " +
			"when `ethermint init` and `ethermint` are invoked respectively",
	}

	// PocketAPIFlag defines the pocket api url
	// #unstable
	PocketAPIFlag = cli.StringFlag{
		Name:  "pocketapi",
		Value: "https://pocketapi.lianxiangcloud.com",
		Usage: "pocket api url",
	}
)

// MigrateFlags sets the global flag from a local flag when it's set.
// This is a temporary function used for migrating old command/flags to the
// new format.
//
// This allows the use of the existing configuration functionality.
// When all flags are migrated this function can be removed and the existing
// configuration functionality must be changed that is uses local flags
func MigrateFlags(action func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				ctx.GlobalSet(name, ctx.String(name))
			}
		}
		return action(ctx)
	}
}
