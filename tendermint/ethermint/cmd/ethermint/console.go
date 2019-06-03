package main

import (
	"fmt"

	"gopkg.in/urfave/cli.v1"
	"github.com/ethereum/go-ethereum/console"
	"github.com/ethereum/go-ethereum/rpc"
	emtUtils "github.com/tendermint/ethermint/cmd/utils"
)

// nolint: gocyclo
func attachCmd(ctx *cli.Context) error {
	endpoint := ctx.Args().First()
	if endpoint == "" {
		return fmt.Errorf("rpc server address needed")
	}

	client, err := rpc.Dial(endpoint)
	if err != nil {
		return err
	}

	ethermintDataDir := emtUtils.MakeDataDir(ctx)

	cfg := console.Config{
		DataDir: ethermintDataDir,
		DocRoot: ".",
		Client:  client,
		Preload: []string{},
	}

	console, err := console.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	// Otherwise print the welcome screen and enter interactive mode
	console.Welcome()
	console.Interactive()

	return nil
}
