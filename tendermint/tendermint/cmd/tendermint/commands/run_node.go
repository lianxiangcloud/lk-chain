package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	nm "github.com/tendermint/tendermint/node"
)

// AddNodeFlags exposes some common configuration options on the command-line
// These are exposed for convenience of commands embedding a tendermint node
func AddNodeFlags(cmd *cobra.Command) {
	// bind flags
	cmd.Flags().String("moniker", config.Moniker, "Node Name")

	// node flags
	cmd.Flags().Bool("fast_sync", config.FastSync, "Fast blockchain syncing")
	cmd.Flags().String("eth_rpcaddr", config.EthRpcAddr, "ethermint RPC listen address")
	cmd.Flags().String("node_type", config.NodeType, "node type (enum{ validator, peer, listener, test })")
	cmd.Flags().String("verify_code", config.VerifyCode, "boss_svr_verify_code, default is 0")
	cmd.Flags().String("log_file", config.Logfile, "output file for rotate logging")
	cmd.Flags().Int64("rollbackblocknumber", config.RollbackBlockNumber, "Rollback block number")

	// abci flags
	cmd.Flags().String("proxy_app", config.ProxyApp, "Proxy app address, or 'nilapp' or 'dummy' for local testing.")
	cmd.Flags().String("abci", config.ABCI, "Specify abci transport (socket | grpc)")

	// rpc flags
	cmd.Flags().String("rpc.laddr", config.RPC.ListenAddress, "RPC listen address. Port required")
	cmd.Flags().String("rpc.grpc_laddr", config.RPC.GRPCListenAddress, "GRPC listen address (BroadcastTx only). Port required")
	cmd.Flags().Bool("rpc.unsafe", config.RPC.Unsafe, "Enabled unsafe rpc methods")

	// p2p flags
	cmd.Flags().String("p2p.laddr", config.P2P.ListenAddress, "Node listen address. (0.0.0.0:0 means any interface, any port)")
	cmd.Flags().String("p2p.seeds", config.P2P.Seeds, "Comma delimited host:port seed nodes")
	cmd.Flags().Bool("p2p.skip_upnp", config.P2P.SkipUPNP, "Skip UPNP configuration")
	cmd.Flags().Bool("p2p.pex", config.P2P.PexReactor, "Enable/disable Peer-Exchange")
	cmd.Flags().String("p2p.bootnodeurl", config.P2P.BootNodeURL, "BootNode Server URL")

	// mempool flags
	cmd.Flags().Float64("mempool.rate_too_many_txs", config.Mempool.RateTooManyTxs, "rate of too many txs in txpool or mempool")

	// consensus flags
	cmd.Flags().Bool("consensus.create_empty_blocks", config.Consensus.CreateEmptyBlocks, "Set this to false to only produce blocks when there are txs or when the AppHash changes")
	cmd.Flags().Int("consensus.create_empty_blocks_interval", config.Consensus.CreateEmptyBlocksInterval, "the interval time between two empty block")
	cmd.Flags().Int("consensus.timeout_commit", config.Consensus.TimeoutCommit, "the interval between blocks in ms(Milliseconds)")
	cmd.Flags().String("consensus.coinbase", config.Consensus.Coinbase, "coinbase")

	//debug
	cmd.Flags().String("debug.cpuprofile", config.Debug.CpuProfile, "Write CPU profile to the given file")
	cmd.Flags().String("debug.trace", config.Debug.Trace, "Write execution trace to the given file")
	cmd.Flags().Bool("debug.pprof", config.Debug.Pprof, "Enable the pprof HTTP server")
	cmd.Flags().Int("debug.pprofport", config.Debug.Pprofport, "pprof HTTP server listening port")
	cmd.Flags().String("debug.pprofaddr", config.Debug.Pprofaddr, "pprof HTTP server listening interface")
}

// NewRunNodeCmd returns the command that allows the CLI to start a
// node. It can be used with a custom PrivValidator and in-process ABCI application.
func NewRunNodeCmd(nodeProvider nm.NodeProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Run the tendermint node",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create & start node
			n, err := nodeProvider(config, logger)
			if err != nil {
				return fmt.Errorf("Failed to create node: %v", err)
			}

			if err := n.Start(); err != nil {
				return fmt.Errorf("Failed to start node: %v", err)
			} else {
				logger.Info("Started node", "nodeInfo", n.Switch().NodeInfo())
			}

			// Trap signal, run forever.
			n.RunForever()
			return nil
		},
	}

	AddNodeFlags(cmd)
	return cmd
}
