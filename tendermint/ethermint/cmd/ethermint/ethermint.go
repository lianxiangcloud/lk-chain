package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"path/filepath"

	"gopkg.in/urfave/cli.v1"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/console"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/tendermint/ethermint/ethereum"
	"github.com/tendermint/abci/server"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/tendermint/ethermint/strategies/validators"
	"github.com/tendermint/ethermint/types"
	cmn "github.com/tendermint/tmlibs/common"
	abciApp "github.com/tendermint/ethermint/app"
	emtUtils "github.com/tendermint/ethermint/cmd/utils"
	ethUtils "github.com/ethereum/go-ethereum/cmd/utils"

	"third_part/reporttrans"
	"third_part/txmgr/service"
	lkdb "third_part/txmgr/db"
)

func ethermintCmd(ctx *cli.Context) error {
	// Step 0: Set go-ethereum output config
	//emtUtils.Setup(ctx)

	// Step 1: set up configuration, including but not limiting to otconfig and otconfmng
	otInit(ctx)

	// Step 2: Setup the go-ethereum node and start it
	node := emtUtils.MakeFullNode(ctx)
	startNode(ctx, node)

	// Step 3: If we can invoke `tendermint node`, let's do so
	// in order to make ethermint as self contained as possible.
	// See Issue https://github.com/tendermint/ethermint/issues/244
	canInvokeTendermintNode := canInvokeTendermint(ctx)
	if canInvokeTendermintNode {
		tendermintHome := tendermintHomeFromEthermint(ctx)
		tendermintArgs := []string{"--home", tendermintHome, "node"}
		go func() {
			if _, err := invokeTendermintNoTimeout(tendermintArgs...); err != nil {
				// We shouldn't go *Fatal* because
				// `tendermint node` might have already been invoked.
				log.Info("tendermint init", "error", err)
			} else {
				log.Info("Successfully invoked `tendermint node`", "args", tendermintArgs)
			}
		}()
		pauseDuration := 3 * time.Second
		log.Info(fmt.Sprintf("Invoked `tendermint node` sleeping for %s", pauseDuration), "args", tendermintArgs)
		time.Sleep(pauseDuration)
	}

	// Setup the ABCI server and start it
	addr := ctx.GlobalString(emtUtils.ABCIAddrFlag.Name)
	abci := ctx.GlobalString(emtUtils.ABCIProtocolFlag.Name)

	// Fetch the registered service of this type
	var backend *ethereum.Backend
	if err := node.Service(&backend); err != nil {
		ethUtils.Fatalf("ethereum backend service not running: %v", err)
	}

	// start lk rpc server
	if ctx.GlobalIsSet(emtUtils.LKRPCAddrFlag.Name) {
		apis, err := service.GetAPIs(backend.Ethereum())
		if err != nil {
			ethUtils.Fatalf("[lk-init] GetAPIs: failed, err=%v", err)
		}

		var (
			network  string   = "tcp"
			origin   []string = []string{"*"}
			endPoint string   = ctx.GlobalString(emtUtils.LKRPCAddrFlag.Name)
		)

		lkWSServer, err := service.StartWSServer(network, endPoint, origin, apis)
		if err != nil {
			ethUtils.Fatalf("[lk-init] StartWSServer: failed, endPoint=%s, err=%v", endPoint, err)
		}
		log.Info("[lk-init] StartWSServer: successs", "info", lkWSServer.Info())
	}

	// In-proc RPC connection so ABCI.Query can be forwarded over the ethereum rpc
	rpcClient, err := node.Attach()
	if err != nil {
		ethUtils.Fatalf("Failed to attach to the inproc geth: %v", err)
	}

	strategy := &types.Strategy{}
	strategy.ValidatorsStrategy = validatorStrategies.NewValidatorsStrategy()

	interval := ctx.GlobalInt(emtUtils.LKNoBlockCreateIntervalFlag.Name)
	if interval <= 0 {
		interval = 5
	}
	log.Info("info", "interval", interval)

	// Create the ABCI app
	ethApp, err := abciApp.NewEthermintApplication(backend, rpcClient, strategy, interval)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ethApp.SetLogger(emtUtils.EthermintLogger().With("module", "ethermint"))

	// Start the app on the ABCI server
	srv, err := server.NewServer(addr, abci, ethApp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	srv.SetLogger(emtUtils.EthermintLogger().With("module", "abci-server"))

	if err := srv.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cmn.TrapSignal(func() {
		srv.Stop()
	})

	return nil
}

// nolint
// startNode copies the logic from go-ethereum
func startNode(ctx *cli.Context, stack *ethereum.Node) {
	emtUtils.StartNode(stack)

	// Unlock any account specifically requested
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)

	passwords := ethUtils.MakePasswordList(ctx)
	unlocks := strings.Split(ctx.GlobalString(ethUtils.UnlockedAccountFlag.Name), ",")
	for i, account := range unlocks {
		if trimmed := strings.TrimSpace(account); trimmed != "" {
			unlockAccount(ctx, ks, trimmed, i, passwords)
		}
	}
	// Register wallet event handlers to open and auto-derive wallets
	events := make(chan accounts.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	go func() {
		// Create an chain state reader for self-derivation
		rpcClient, err := stack.Attach()
		if err != nil {
			ethUtils.Fatalf("Failed to attach to self: %v", err)
		}
		stateReader := ethclient.NewClient(rpcClient)

		// Open and self derive any wallets already attached
		for _, wallet := range stack.AccountManager().Wallets() {
			if err := wallet.Open(""); err != nil {
				log.Warn("Failed to open wallet", "url", wallet.URL(), "err", err)
			} else {
				wallet.SelfDerive(accounts.DefaultBaseDerivationPath, stateReader)
			}
		}
		// Listen for wallet event till termination
		for event := range events {
			if event.Kind == accounts.WalletArrived {
				if err := event.Wallet.Open(""); err != nil {
					log.Warn("New wallet appeared, failed to open", "url", event.Wallet.URL(), "err", err)
				} else {
					status, _ := event.Wallet.Status()
					log.Info("New wallet appeared", "url", event.Wallet.URL(), "status", status)
					event.Wallet.SelfDerive(accounts.DefaultBaseDerivationPath, stateReader)
				}
			} else {
				log.Info("Old wallet dropped", "url", event.Wallet.URL())
				event.Wallet.Close()
			}
		}
	}()
}

// tries unlocking the specified account a few times.
// nolint: unparam
func unlockAccount(ctx *cli.Context, ks *keystore.KeyStore, address string, i int, passwords []string) (accounts.Account, string) {
	account, err := ethUtils.MakeAddress(ks, address)
	if err != nil {
		ethUtils.Fatalf("Could not list accounts: %v", err)
	}
	for trials := 0; trials < 3; trials++ {
		prompt := fmt.Sprintf("Unlocking account %s | Attempt %d/%d", address, trials+1, 3)
		password := getPassPhrase(prompt, false, i, passwords)
		err = ks.Unlock(account, password)
		if err == nil {
			log.Info("Unlocked account", "address", account.Address.Hex())
			return account, password
		}
		if err, ok := err.(*keystore.AmbiguousAddrError); ok {
			log.Info("Unlocked account", "address", account.Address.Hex())
			return ambiguousAddrRecovery(ks, err, password), password
		}
		if err != keystore.ErrDecrypt {
			// No need to prompt again if the error is not decryption-related.
			break
		}
	}
	// All trials expended to unlock account, bail out
	ethUtils.Fatalf("Failed to unlock account %s (%v)", address, err)

	return accounts.Account{}, ""
}

// getPassPhrase retrieves the passwor associated with an account, either fetched
// from a list of preloaded passphrases, or requested interactively from the user.
// nolint: unparam
func getPassPhrase(prompt string, confirmation bool, i int, passwords []string) string {
	// If a list of passwords was supplied, retrieve from them
	if len(passwords) > 0 {
		if i < len(passwords) {
			return passwords[i]
		}
		return passwords[len(passwords)-1]
	}
	// Otherwise prompt the user for the password
	if prompt != "" {
		fmt.Println(prompt)
	}
	password, err := console.Stdin.PromptPassword("Passphrase: ")
	if err != nil {
		ethUtils.Fatalf("Failed to read passphrase: %v", err)
	}
	if confirmation {
		confirm, err := console.Stdin.PromptPassword("Repeat passphrase: ")
		if err != nil {
			ethUtils.Fatalf("Failed to read passphrase confirmation: %v", err)
		}
		if password != confirm {
			ethUtils.Fatalf("Passphrases do not match")
		}
	}
	return password
}

func ambiguousAddrRecovery(ks *keystore.KeyStore, err *keystore.AmbiguousAddrError, auth string) accounts.Account {
	fmt.Printf("Multiple key files exist for address %x:\n", err.Addr)
	for _, a := range err.Matches {
		fmt.Println("  ", a.URL)
	}
	fmt.Println("Testing your passphrase against all of them...")
	var match *accounts.Account
	for _, a := range err.Matches {
		if err := ks.Unlock(a, auth); err == nil {
			match = &a
			break
		}
	}
	if match == nil {
		ethUtils.Fatalf("None of the listed files could be unlocked.")
	}
	fmt.Printf("Your passphrase unlocked %s\n", match.URL)
	fmt.Println("In order to avoid this warning, you need to remove the following duplicate key files:")
	for _, a := range err.Matches {
		if a != *match {
			fmt.Println("  ", a.URL)
		}
	}
	return *match
}

// otInit sets up everything you want to initialize before the node is being started
func otInit(ctx *cli.Context) {

	ethermintDataDir := emtUtils.MakeDataDir(ctx)
	var chainDBPath string = filepath.Join(ethermintDataDir, "ethermint/chaindata")
	if _, err := os.Stat(chainDBPath); err != nil {
		ethUtils.Fatalf("[lk-init] chaindb not exist, need to init at first, datadir=%s, chaindbpath=%s", ethermintDataDir, chainDBPath)
	}
	chainDb, err := ethdb.NewLDBDatabase(chainDBPath, 0, 0)
	if err != nil {
		ethUtils.Fatalf("[lk-init] could not open database: chainDBPath=%s, %v", err)
	}
	chainDb.Close()

	// set up warning rate of too many txs in txpool or mempool
	if ctx.GlobalIsSet(emtUtils.LKRateTooManyTxsFlag.Name) {
		rate := ctx.GlobalFloat64(emtUtils.LKRateTooManyTxsFlag.Name)
		reporttrans.SetTxsWarnRate(rate)
		log.Info("[lk-init] set up warning rate for too many txs in txpool or mempool", "rate", rate)
	}
	if ctx.GlobalIsSet(emtUtils.LKNoBlockWarnIntervalFlag.Name) {
		interval := ctx.GlobalInt(emtUtils.LKNoBlockWarnIntervalFlag.Name)
		reporttrans.SetWarnInterval(interval)
		log.Info("[lk-init] init set no block warn interval ", "seconds", interval)
	} else {
		log.Info("[lk-init] init set no block warn interval to default 300 seconds")
	}
	reporttrans.RunCheck()

	// set up lk leveldb
	txDBPath := filepath.Join(ethermintDataDir, "ethermint/txdata")
	if err := lkdb.Init(txDBPath); err != nil {
		ethUtils.Fatalf("[lk-init] init lkdb: failed, path=%s, err=%v", txDBPath, err)
	}
}
