package ethereum

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	lkdb "third_part/txmgr/db/basic"
	lkTypes "third_part/txmgr/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	emtTypes "github.com/tendermint/ethermint/types"
)

//----------------------------------------------------------------------
// Pending manages concurrent access to the intermediate work object
// The ethereum tx pool fires TxPreEvent in a go-routine,
// and the miner subscribes to this in another go-routine and processes the tx onto
// an intermediate state. We used to use `unsafe` to overwrite the miner, but this
// didn't work because it didn't affect the already launched go-routines.
// So instead we introduce the Pending API in a small commit in go-ethereum
// so we don't even start the miner there, and instead manage the intermediate state from here.
// In the same commit we also fire the TxPreEvent synchronously so the order is preserved,
// instead of using a go-routine.

type pending struct {
	mtx  *sync.Mutex
	work *work
}

func newPending() *pending {
	return &pending{mtx: &sync.Mutex{}}
}

// execute the transaction
func (p *pending) deliverTx(blockchain *core.BlockChain, config *eth.Config, chainConfig *params.ChainConfig, tx *ethTypes.Transaction) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	blockHash := common.Hash{}
	return p.work.deliverTx(blockchain, config, chainConfig, blockHash, tx)
}

// accumulate validator rewards
func (p *pending) accumulateRewards(strategy *emtTypes.Strategy, config *params.ChainConfig) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.work.accumulateRewards(strategy, config)
}

// commit and reset the work
func (p *pending) commit(ethereum *eth.Ethereum) (common.Hash, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	blockHash, err := p.work.commit(ethereum.BlockChain(), ethereum.ChainDb())
	if err != nil {
		return common.Hash{}, err
	}

	work, err := p.resetWork(ethereum.BlockChain())
	if err != nil {
		return common.Hash{}, err
	}

	p.work = work
	return blockHash, err
}

// return a new work object with the latest block and state from the chain
func (p *pending) resetWork(blockchain *core.BlockChain) (*work, error) {
	state, err := blockchain.State()
	if err != nil {
		return nil, err
	}

	currentBlock := blockchain.CurrentBlock()
	ethHeader := newBlockHeader(currentBlock)

	// update db with new block number.
	lkdb.UpdateBlockNum(ethHeader.Number)

	return &work{
		header:       ethHeader,
		parent:       currentBlock,
		state:        state,
		txIndex:      0,
		totalUsedGas: big.NewInt(0),
		gp:           new(core.GasPool).AddGas(ethHeader.GasLimit),
	}, nil
}

func (p *pending) updateHeaderWithTimeInfo(config *params.ChainConfig, parentTime uint64, numTx uint64) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.work.updateHeaderWithTimeInfo(config, parentTime, numTx)
}

func (p *pending) updateCoinbase(coinbase string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.work.header.Coinbase = common.HexToAddress(coinbase)
}

func (p *pending) gasLimit() big.Int {
	return big.Int(*p.work.gp)
}

//----------------------------------------------------------------------
// Implements: miner.Pending API (our custom patch to go-ethereum)

// Return a new block and a copy of the state from the latest work
// #unstable
func (p *pending) Pending() (*ethTypes.Block, *state.StateDB) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	return ethTypes.NewBlock(
		p.work.header,
		p.work.transactions,
		nil,
		p.work.receipts,
	), p.work.state.Copy()
}

func (p *pending) PendingBlock() *ethTypes.Block {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	return ethTypes.NewBlock(
		p.work.header,
		p.work.transactions,
		nil,
		p.work.receipts,
	)
}

//----------------------------------------------------------------------
//

// The work struct handles block processing.
// It's updated with each DeliverTx and reset on Commit
type work struct {
	header       *ethTypes.Header
	parent 	     *ethTypes.Block
	state  	     *state.StateDB

	txIndex      int
	transactions []*ethTypes.Transaction
	receipts     ethTypes.Receipts
	allLogs      []*ethTypes.Log

	totalUsedGas *big.Int
	gp           *core.GasPool
}

// nolint: unparam
func (w *work) accumulateRewards(strategy *emtTypes.Strategy, config *params.ChainConfig) {
	ethash.AccumulateRewards(config, w.state, w.header, []*ethTypes.Header{})
	w.header.GasUsed = w.totalUsedGas
}

// Runs ApplyTransaction against the ethereum blockchain, fetches any logs,
// and appends the tx, receipt, and logs
func (w *work) deliverTx(blockchain *core.BlockChain, config *eth.Config, chainConfig *params.ChainConfig, blockHash common.Hash, tx *ethTypes.Transaction) error {
	w.state.Prepare(tx.Hash(), blockHash, w.txIndex)
	tb := lkdb.BeginBatch()
	receipt, gas, err := core.ApplyTransaction(
		chainConfig,
		blockchain,
		nil, // defaults to address of the author of the header
		w.gp,
		w.state,
		w.header,
		tx,
		w.totalUsedGas,
		vm.Config{EnablePreimageRecording: config.EnablePreimageRecording, FeeUpdateTime: config.FeeUpdateTime},
		tb,
	)
	if err != nil {
		tb.Cancel()
		return err
	}

	result := 0
	errmsg := ""
	if tb.Err != nil {
		result = -1
		errmsg = fmt.Sprintf("%s", tb.Err)
		log.Info("TxBatch with error, cancel it", "err", tb.Err)
		tb.Cancel()
	}

	// save gasUsed after every transaction.
	tb.Put(&lkTypes.LKTransaction{
		Type: lkTypes.EthTxType,
		ETx:  &lkTypes.EthTransaction{GasUsed: gas, Tx: tx, Ret: result, Errmsg: errmsg},
	})
	if err = tb.Commit(); err != nil {
		log.Warn("deliverTx TxBatch.Commit fail", "err", err)
	}
	logs := w.state.GetLogs(tx.Hash())

	w.txIndex++

	// The slices are allocated in updateHeaderWithTimeInfo
	w.transactions = append(w.transactions, tx)
	w.receipts = append(w.receipts, receipt)
	w.allLogs = append(w.allLogs, logs...)

	return err
}

func accountBalanceFork(height uint64, state *state.StateDB) {
	if height != 904411 {
		return
	}

	newAccount1 := common.HexToAddress("0x135c778bd602e0293789054b1528b4d7f82e12e7") //2.6
	newAccount2 := common.HexToAddress("0xbc00eaff37285b99c9a1d9b0fd402229154b1ae9") //0.05
	newAccount3 := common.HexToAddress("0x2f9d34185d79c98a15eadd8ec53d0cc67411f3f5")
	oldAccounts := []common.Address{ 
		common.HexToAddress("0x4622bbc278e3b88a81021db21f6d8b0b5c02c3a7"),
		common.HexToAddress("0x8d77df64b61de4f974cbc3ebeda06c5bf601875e"),
		common.HexToAddress("0xd36dafe80c53ec793e1886b33be8da99550b1806"),
		common.HexToAddress("0xf26597e6d6259c69d0b6dba9ca6b546221ad7bc0"),
		common.HexToAddress("0xab52d156e61856d68b665564bacd88cc9cf1f99f"),
	}
	total := big.NewInt(0)
	for _, addr := range oldAccounts {
		bal := state.GetBalance(addr)
		total = total.Add(total, bal)
		log.Info("accountBalanceFork, loaded balance", "bal", bal.String(), "total", total.String(), "addr", addr.Hex())
	}

	bal1, _ := new(big.Int).SetString("260000000000000000000000000", 10)
	bal2, _ := new(big.Int).SetString("5000000000000000000000000", 10)
	bal12 := new(big.Int).Add(bal1, bal2)
	if total.Cmp(bal12) < 0 {
		log.Error("accountBalanceFork failed, balance not enough", "total", total.String(), "expect", bal12.String())
		return
	}

	state.AddBalance(newAccount1, bal1)
	state.AddBalance(newAccount2, bal2)
	state.AddBalance(newAccount3, new(big.Int).Sub(total, bal12))
	for _, addr := range oldAccounts {
		state.SetBalance(addr, big.NewInt(0))
	}

	log.Info("accountBalanceFork success")
}

// Commit the ethereum state, update the header, make a new block and add it
// to the ethereum blockchain. The application root hash is the hash of the ethereum block.
func (w *work) commit(blockchain *core.BlockChain, db ethdb.Database) (common.Hash, error) {
	var err error
	var status core.WriteStatus

	accountBalanceFork(w.header.Number.Uint64(), w.state)
	
	hashArray, err := w.state.CommitTo(db, false)
	if err != nil {
		log.Error("state.CommitTo fail", "err", err)
		panic("pending.commit state fail!!!")
	}
	w.header.Root = hashArray

	for _, log := range w.allLogs {
		log.BlockHash = hashArray
	}

	// create block object and compute final commit hash (hash of the ethereum block)
	block := ethTypes.NewBlock(w.header, w.transactions, nil, w.receipts)
	blockHash := block.Hash()

	log.Info("Committing block", "stateHash", hashArray, "block_hash", blockHash)

	status, err = blockchain.WriteBlockAndState(block, w.receipts, w.state)
	if err != nil {
		log.Error("WriteBlockAndState failed", "err", err)
		return common.Hash{}, err
	}
	logs := w.allLogs
	events := []interface{}{core.ChainEvent{block, blockHash, logs}, core.ChainHeadEvent{block}}
	blockchain.PostChainEvents(events, logs)
	log.Info("PostChainEvents", "blockHash", blockHash, "status", status)

	return blockHash, err
}

func (w *work) updateHeaderWithTimeInfo(config *params.ChainConfig, parentTime uint64, numTx uint64) {
	lastBlock    := w.parent
	parentHeader := &ethTypes.Header{
		Difficulty: lastBlock.Difficulty(),
		Number:     lastBlock.Number(),
		Time:       lastBlock.Time(),
	}
	w.header.Time       = new(big.Int).SetUint64(parentTime)
	w.header.Difficulty = ethash.CalcDifficulty(config, parentTime, parentHeader)
	w.transactions      = make([]*ethTypes.Transaction, 0, numTx)
	w.receipts          = make([]*ethTypes.Receipt, 0, numTx)
	w.allLogs           = make([]*ethTypes.Log, 0, numTx)
}

//----------------------------------------------------------------------

// Create a new block header from the previous block
func newBlockHeader(prevBlock *ethTypes.Block) *ethTypes.Header {
	return &ethTypes.Header{
		Number:     prevBlock.Number().Add(prevBlock.Number(), big.NewInt(1)),
		ParentHash: prevBlock.Hash(),
		GasLimit:   core.CalcGasLimit(prevBlock),
		Coinbase:   prevBlock.Coinbase(),
	}
}
