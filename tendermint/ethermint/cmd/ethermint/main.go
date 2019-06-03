package main

import (
	"fmt"
	"os"

	ethUtils "github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/params"
	"github.com/tendermint/ethermint/cmd/utils"
	"github.com/tendermint/ethermint/version"
	"gopkg.in/urfave/cli.v1"
)

const (
	ethereumStartFailedCode = 1
)

var (
	// The app that holds all commands and flags.
	app = ethUtils.NewApp(version.Version, "the ethermint command line interface")
	// flags that configure the go-ethereum node
	otFlags = []cli.Flag{
		utils.LKRotateLogFileFlag,
		utils.LKRoleFlag,
		utils.LKRPCAddrFlag,
		utils.LKNoBlockWarnIntervalFlag,
		utils.LKNoBlockCreateIntervalFlag,
		utils.LKOnlineFlag,
		utils.LKRollbackBlockByNumberFlag,
		utils.LKRateTooManyTxsFlag,
		utils.LKStateFlag,
	}
	nodeFlags = []cli.Flag{
		ethUtils.DataDirFlag,
		ethUtils.KeyStoreDirFlag,
		ethUtils.NoUSBFlag,
		// Performance tuning
		ethUtils.CacheFlag,
		ethUtils.TrieCacheGenFlag,
		// Account settings
		ethUtils.UnlockedAccountFlag,
		ethUtils.PasswordFileFlag,
		ethUtils.VMEnableDebugFlag,
		// Logging and debug settings
		ethUtils.NoCompactionFlag,
		// Gas price oracle settings
		ethUtils.GpoBlocksFlag,
		ethUtils.GpoPercentileFlag,
		utils.TargetGasLimitFlag,
		// Gas Price
		ethUtils.GasPriceFlag,
	}

	rpcFlags = []cli.Flag{
		ethUtils.RPCEnabledFlag,
		ethUtils.RPCListenAddrFlag,
		ethUtils.RPCPortFlag,
		ethUtils.RPCCORSDomainFlag,
		ethUtils.RPCApiFlag,
		ethUtils.IPCDisabledFlag,
		ethUtils.WSEnabledFlag,
		ethUtils.WSListenAddrFlag,
		ethUtils.WSPortFlag,
		ethUtils.WSApiFlag,
		ethUtils.WSAllowedOriginsFlag,
		ethUtils.PocketAPIFlag,
	}

	// flags that configure the ABCI app
	ethermintFlags = []cli.Flag{
		utils.TendermintAddrFlag,
		utils.ABCIAddrFlag,
		utils.ABCIProtocolFlag,
		utils.VerbosityFlag,
		utils.ConfigFileFlag,
		utils.WithTendermintFlag,
	}
)

func init() {
	app.Action = ethermintCmd
	app.HideVersion = true
	app.Commands = []cli.Command{
		{
			Action:      utils.MigrateFlags(initCmd),
			Name:        "init",
			Usage:       "init genesis.json",
			Description: "Initialize the files",
			Flags:       append(app.Flags, otFlags...),
		},
		{
			Action:      versionCmd,
			Name:        "version",
			Usage:       "",
			Description: "Print the version",
		},
		{
			Action: resetCmd,
			Name:   "unsafe_reset_all",
			Usage:  "(unsafe) Remove ethermint database",
		},
		{
			Action:      attachCmd,
			Name:        "attach",
			Usage:       "ethermint attach http://127.0.0.1:16000",
			Description: "Start an interactive JavaScript environment (connect to node)",
		},
		{
			Action:      utils.MigrateFlags(rollbackCmd),
			Name:        "rollback",
			Usage:       "ethermint rollback --lkrollbackbyblocknumber 10000",
			Description: "rollback to the height",
			Flags:       []cli.Flag{utils.LKRollbackBlockByNumberFlag, ethUtils.DataDirFlag},
		},
	}

	app.Flags = append(app.Flags, otFlags...)
	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, ethermintFlags...)

	app.Before = func(ctx *cli.Context) error {
		if err := utils.Setup(ctx); err != nil {
			return err
		}
		ethUtils.SetupNetwork(ctx)
		return nil
	}
}

func versionCmd(ctx *cli.Context) error {
	fmt.Println("ethermint: ", version.Version)
	fmt.Println("go-ethereum: ", params.Version)
	return nil
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ethereumStartFailedCode)
	}
}
