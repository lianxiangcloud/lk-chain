package ethereum

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rpc"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	emtTypes "github.com/tendermint/ethermint/types"
	abciTypes "github.com/tendermint/abci/types"
	rpcClient "github.com/tendermint/tendermint/rpc/lib/client"

	"third_part/reporttrans"
)

//----------------------------------------------------------------------
// Backend manages the underlying ethereum state for storage and processing,
// and maintains the connection to Tendermint for forwarding txs

// Backend handles the chain database and VM
// #stable - 0.4.0
type Backend struct {
	// backing ethereum structures
	config   *eth.Config
	ethereum *eth.Ethereum

	// pending ...
	pending *pending

	// client for forwarding txs to Tendermint
	client rpcClient.HTTPClient
}

// NewBackend creates a new Backend
// #stable - 0.4.0
func NewBackend(ctx *node.ServiceContext, config *eth.Config, client rpcClient.HTTPClient) (*Backend, error) {
	p := newPending()

	// eth.New takes a ServiceContext for the EventMux, the AccountManager,
	// and some basic functions around the DataDir.
	ethereum, err := eth.New(ctx, config, p)
	if err != nil {
		return nil, err
	}

	// send special event to go-ethereum to switch homestead=true
	currentBlock := ethereum.BlockChain().CurrentBlock()
	ethereum.EventMux().Post(core.ChainHeadEvent{currentBlock}) // nolint: vet, errcheck

	// We don't need PoW/Uncle validation
	ethereum.BlockChain().SetValidator(NullBlockProcessor{})

	ethBackend := &Backend{
		ethereum: ethereum,
		pending:  p,
		client:   client,
		config:   config,
	}
	return ethBackend, nil
}

func (b *Backend) IsEtxExist(hash common.Hash) bool {
	if tx, _, _, _ := core.GetTransaction(b.ethereum.ChainDb(), hash); tx != nil {
		return true
	}
	return false
}

func (b *Backend) GetTransactionByHash(hash common.Hash) *ethTypes.Transaction {
	tx, _, _, _ := core.GetTransaction(b.ethereum.ChainDb(), hash)
	return tx
}

// Ethereum returns the underlying the ethereum object
// #stable
func (b *Backend) Ethereum() *eth.Ethereum {
	return b.ethereum
}

// Config returns the eth.Config
// #stable
func (b *Backend) Config() *eth.Config {
	return b.config
}

func ReportDeliverTx(tx *ethTypes.Transaction) {
	reporttrans.UpdateDeliverTx()
	var reportData reporttrans.TransactionReportData
	var signer ethTypes.Signer = ethTypes.FrontierSigner{}
	if tx.Protected() {
		signer = ethTypes.NewEIP155Signer(tx.ChainId())
	}
	// Make sure the transaction is signed properly
	from, err := ethTypes.Sender(signer, tx)
	if err != nil {
	//	log.Error("[lktx] deliverTx failed:", "hash", tx.Hash(), "err", core.ErrInvalidSender)
		reportData.Result = reporttrans.ERR_SENDER_INVALID
		reporttrans.Report(reportData)
		return
	}

	reportData.From = from.Hex()
	if tx.To() != nil {
		reportData.To = tx.To().String()
	}
	reportData.Cost = tx.Cost().String()
	reportData.Gas = tx.Gas().String()
	reportData.Nonce = tx.Nonce()
	reportData.Result = reporttrans.ERR_DELIVER_TX_SUCC
	reportData.Hash = tx.Hash().String()
	reporttrans.Report(reportData)
}

//----------------------------------------------------------------------
// Handle block processing

// DeliverTx appends a transaction to the current block
// #stable
func (b *Backend) DeliverTx(tx *ethTypes.Transaction) error {
	ReportDeliverTx(tx)
	return b.pending.deliverTx(b.ethereum.BlockChain(), b.config, b.ChainConfig(), tx)
}

// AccumulateRewards accumulates the rewards based on the given strategy
// #unstable
func (b *Backend) AccumulateRewards(strategy *emtTypes.Strategy) {
	b.pending.accumulateRewards(strategy, b.ChainConfig())
}

func (b *Backend) ChainConfig() *params.ChainConfig {
	return b.ethereum.BlockChain().Config()
}

// Commit finalises the current block
// #unstable
func (b *Backend) Commit() (common.Hash, error) {
	return b.pending.commit(b.ethereum)
}

// ResetWork resets the current block to a fresh object
// #unstable
func (b *Backend) ResetWork() error {
	work, err := b.pending.resetWork(b.ethereum.BlockChain())
	b.pending.work = work
	return err
}

// UpdateHeaderWithTimeInfo uses the tendermint header to update the ethereum header
// #unstable
func (b *Backend) UpdateHeader(tmHeader *abciTypes.Header) {
	b.pending.updateCoinbase(tmHeader.Coinbase)
	b.pending.updateHeaderWithTimeInfo(b.ChainConfig(), uint64(tmHeader.Time), uint64(tmHeader.GetNumTxs()))
}

// GasLimit returns the maximum gas per block
// #unstable
func (b *Backend) GasLimit() big.Int {
	return b.pending.gasLimit()
}

//----------------------------------------------------------------------
// Implements: node.Service

// APIs returns the collection of RPC services the ethereum package offers.
// #stable - 0.4.0
func (b *Backend) APIs() []rpc.API {
	apis := b.Ethereum().APIs()
	retApis := []rpc.API{}
	for _, v := range apis {
		if v.Namespace == "net" {
			v.Service = NewNetRPCService(b.config.NetworkId)
		}
		if v.Namespace == "miner" {
			continue
		}
		if _, ok := v.Service.(*eth.PublicMinerAPI); ok {
			continue
		}
		retApis = append(retApis, v)
	}
	return retApis
}

// Start implements node.Service, starting all internal goroutines needed by the
// Ethereum protocol implementation.
// #stable
func (b *Backend) Start(svr *p2p.Server) error {
	b.ethereum.Start(svr)
	go b.txBroadcastLoop()
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Ethereum protocol.
// #stable
func (b *Backend) Stop() error {
	b.ethereum.Stop() // nolint: errcheck
	return nil
}

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
// #stable
func (b *Backend) Protocols() []p2p.Protocol {
	return nil
}

//----------------------------------------------------------------------
// We need a block processor that just ignores PoW and uncles and so on

// NullBlockProcessor does not validate anything
// #unstable
type NullBlockProcessor struct{}

// ValidateBody does not validate anything
// #unstable
func (NullBlockProcessor) ValidateBody(*ethTypes.Block) error { return nil }

// ValidateState does not validate anything
// #unstable
func (NullBlockProcessor) ValidateState(block, parent *ethTypes.Block, state *state.StateDB, receipts ethTypes.Receipts, usedGas *big.Int) error {
	return nil
}
