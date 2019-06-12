package app

import (
	"fmt"
	"time"
	"math/big"
	"encoding/json"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/tendermint/ethermint/ethereum"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	abci "github.com/tendermint/abci/types"
	abciTypes "github.com/tendermint/abci/types"
	emtTypes "github.com/tendermint/ethermint/types"
	tmLog "github.com/tendermint/tmlibs/log"

	"third_part/lklog"
	"third_part/reporttrans"
	"third_part/txmgr/events"
	lkdb "third_part/txmgr/db/basic"
	lkTypes "third_part/txmgr/types"
)

// EthermintApplication implements an ABCI application
// #stable - 0.4.0
type EthermintApplication struct {
	// backend handles the ethereum state machine
	// and wrangles other services started by an ethereum node (eg. tx pool)
	backend *ethereum.Backend // backend ethereum struct

	// a closure to return the latest current state from the ethereum blockchain
	getCurrentState func() (*state.StateDB, error)
	checkTxState *state.StateDB

	// an ethereum rpc client we can forward queries to
	rpcClient   *rpc.Client

	// strategy for validator compensation
	strategy *emtTypes.Strategy
	logger   tmLog.Logger

	// for monitor by renyakun
	ts             *time.Timer
	timeMsg        chan string
	continueBlock  bool
	reportInterval int

	nonceMap       map[common.Address]uint64
}

var _ abci.Application = &EthermintApplication{}

// NewEthermintApplication creates a fully initialised instance of EthermintApplication
// #stable - 0.4.0
func NewEthermintApplication(backend *ethereum.Backend,
	client *rpc.Client, strategy *emtTypes.Strategy, interval int) (*EthermintApplication, error) {

	state, err := backend.Ethereum().BlockChain().State()
	if err != nil {
		return nil, err
	}

	app := &EthermintApplication{
		backend:           backend,
		rpcClient:         client,
		getCurrentState:   backend.Ethereum().BlockChain().State,
		checkTxState:      state.Copy(),
		strategy:          strategy,
		ts:                time.NewTimer(time.Second * time.Duration(interval)),
		timeMsg:           make(chan string),
		continueBlock:     false,
		nonceMap:          make(map[common.Address]uint64),
	}

	if err := app.backend.ResetWork(); err != nil {
		return nil, err
	}
	app.reportInterval = interval
	go app.TimerLoop()

	return app, nil
}

func (app *EthermintApplication) TimerLoop() {
	for {
		select {
		case msg := <-app.timeMsg:
			app.HandlerTimerMsg(msg)
		case <-app.ts.C:
			app.HandlerTimeOut()
		}
	}
	app.ts.Stop()
}

func (app *EthermintApplication) HandlerTimeOut() {
	if app.continueBlock == false {
		lklog.Warn("1030005\x01NoBlockCreate")
	}
	app.logger.Debug("Info", "handletimeout", app.continueBlock)
	app.continueBlock = false
	app.ts.Reset(time.Second * time.Duration(app.reportInterval))
}

func (app *EthermintApplication) HandlerTimerMsg(msg string) {
	if msg == "continueBlock" {
		app.continueBlock = true
		app.ts.Reset(time.Second * time.Duration(app.reportInterval))
	}
}

// SetLogger sets the logger for the ethermint application
func (app *EthermintApplication) SetLogger(log tmLog.Logger) {
	app.logger = log
}

var bigZero = big.NewInt(0)

// maxTransactionSize is 32KB in order to prevent DOS attacks
const maxTransactionSize = 32768

// Info returns information about the last height and app_hash to the tendermint engine
// #stable - 0.4.0
func (app *EthermintApplication) Info(req abciTypes.RequestInfo) abciTypes.ResponseInfo {
	blockchain   := app.backend.Ethereum().BlockChain()
	currentBlock := blockchain.CurrentBlock()
	height       := currentBlock.Number()
	hash         := currentBlock.Hash()

	app.logger.Debug("Info", "height", height) // nolint: errcheck

	// This check determines whether it is the first time ethermint gets started.
	// If it is the first time, then we have to respond with an empty hash, since
	// that is what tendermint expects.
	if height.Cmp(bigZero) == 0 {
		return abciTypes.ResponseInfo{
			Data:             "ABCIEthereum",
			LastBlockHeight:  height.Int64(),
			LastBlockAppHash: []byte{},
		}
	}

	return abciTypes.ResponseInfo{
		Data:             "ABCIEthereum",
		LastBlockHeight:  height.Int64(),
		LastBlockAppHash: hash[:],
	}
}

// SetOption sets a configuration option
// #stable - 0.4.0
func (app *EthermintApplication) SetOption(req abciTypes.RequestSetOption) abciTypes.ResponseSetOption {
	app.logger.Info("SetOption", "key", req.Key, "value", req.Value) // nolint: errcheck
	return abciTypes.ResponseSetOption{Code: abciTypes.CodeTypeOK}
}

// Query queries the state of the EthermintApplication
// #stable - 0.4.0
func (app *EthermintApplication) Query(query abciTypes.RequestQuery) abciTypes.ResponseQuery {
	app.logger.Debug("Query") // nolint: errcheck
	var in jsonRequest
	if err := json.Unmarshal(query.Data, &in); err != nil {
		return abciTypes.ResponseQuery{Code: 1, Log: err.Error()}
	}
	var result interface{}
	if err := app.rpcClient.Call(&result, in.Method, in.Params...); err != nil {
		return abciTypes.ResponseQuery{Code: 1, Log: err.Error()}
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return abciTypes.ResponseQuery{Code: 1, Log: err.Error()}
	}
	return abciTypes.ResponseQuery{Code: 0, Value: bytes}
}

// InitChain initializes the validator set
// #stable - 0.4.0
func (app *EthermintApplication) InitChain(req abciTypes.RequestInitChain) abciTypes.ResponseInitChain {
	app.strategy.SetValidators(req.Validators)
	app.logger.Info("InitChain", "Validators", app.strategy.GetCurrentValidators())
	return abciTypes.ResponseInitChain{}
}

// CheckTx checks a transaction is valid but does not mutate the state
// #stable - 0.4.0
func (app *EthermintApplication) CheckTx(txBytes []byte) abciTypes.ResponseCheckTx {
	jsonBytes, err := hexutil.Decode(string(txBytes))
	if err != nil {
		app.logger.Error("CheckTx: Received Invalid transaction", "error", err.Error(), "txBytes", string(txBytes))
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  err.Error(),
		}
	}
	var etherTx lkTypes.EtherTx
	err = json.Unmarshal(jsonBytes, &etherTx)
	if err != nil {
		app.logger.Error("CheckTx: Received Invalid transaction", "error", err.Error(), "etherTx", string(jsonBytes))
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  err.Error(),
		}
	}
	switch etherTx.Type {
	case "tx":
		tx, err := decodeTxString(etherTx.Tx)
		if err != nil {
			app.logger.Error("CheckTx: Received Invalid transaction", "error", err.Error(), "tx", etherTx.Tx)
			return abciTypes.ResponseCheckTx{
				Code: 1,
				Log:  err.Error(),
			}
		}
		app.logger.Info("CheckTx: Received valid transaction", "tx", tx) // nolint: errcheck
		return app.validateEthTx(tx, "tx")
	case "val":
		if _, err := app.strategy.CheckValidatorTx([]byte(etherTx.Tx)); err != nil {
			app.logger.Error("CheckTx: Received Invalid ValidatorTx", "error", err.Error(), "tx", etherTx.Tx)
			return abciTypes.ResponseCheckTx{
				Code: 1,
				Log:  "Invalid ValidatorTx",
			}
		}
		app.logger.Info("CheckTx: Received valid ValidatorTx", "tx", string(etherTx.Tx)) // nolint: errcheck
		return abciTypes.ResponseCheckTx{
			Code: 0,
		}
	case lkTypes.LKTxType:
		lkx := &lkTypes.LKTransaction{}
		err := lkx.Decode(etherTx.Tx)
		if err != nil {
			app.logger.Debug("CheckTx: Decode LKTransaction", "err", err, "tx", etherTx.Tx)
			return abciTypes.ResponseCheckTx{
				Code: 1,
				Log:  "Decode LKTransaction error",
			}
		}
		app.logger.Debug("CheckTx: Received LKTransaction", "type", lkx.Type)

		if lkx.Type != lkTypes.EthTxType {
			app.logger.Info("CheckTx: invalid tx type", "type", lkx.Type)
			return abciTypes.ResponseCheckTx{
				Code: 1,
				Log:  "invalid tx type",
			}
		}

		return app.checkLKTransaction(lkx, lkTypes.LKTxType)
	default:
		app.logger.Error("CheckTx: Received Invalid Tx", "type", etherTx.Type, "etherTx", string(jsonBytes))
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  "Unknown Type",
		}
	}
}

// DeliverTx executes a transaction against the latest state
// #stable - 0.4.0
func (app *EthermintApplication) DeliverTx(txBytes []byte) abciTypes.ResponseDeliverTx {
	jsonBytes, err := hexutil.Decode(string(txBytes))
	if err != nil {
		app.logger.Error("DeliverTx: Received Invalid transaction", "error", err.Error(), "txBytes", string(txBytes))
		return abciTypes.ResponseDeliverTx{
			Code: 1,
			Log:  err.Error(),
		}
	}
	var etherTx lkTypes.EtherTx
	err = json.Unmarshal(jsonBytes, &etherTx)
	if err != nil {
		app.logger.Error("DeliverTx: Received Invalid transaction", "error", err.Error(), "etherTx", string(jsonBytes))
		return abciTypes.ResponseDeliverTx{
			Code: 1,
			Log:  err.Error(),
		}
	}

	switch etherTx.Type {
	case "tx":
		tx, err := decodeTxString(etherTx.Tx)
		if err != nil {
			app.logger.Error("DeliverTx: Received Invalid transaction", "error", err.Error(), "tx", etherTx.Tx)
			return abciTypes.ResponseDeliverTx{
				Code: 1,
				Log:  err.Error(),
			}
		}
		app.logger.Info("DeliverTx: Received valid transaction", "tx", tx, "hash", tx.Hash().String()) // nolint: errcheck
		err = app.backend.DeliverTx(tx)
		if err != nil {
			app.logger.Error("DeliverTx: Error delivering tx to ethereum backend", "tx", tx, "hash", tx.Hash().String(), "err", err) // nolint: errcheck
			return abciTypes.ResponseDeliverTx{
				Code: 1,
				Log:  err.Error(),
			}
		}
		return abciTypes.ResponseDeliverTx{
			Code: abciTypes.CodeTypeOK,
		}
	case "val":
		if err := app.strategy.CollectTx([]byte(etherTx.Tx)); err != nil {
			app.logger.Error("DeliverTx: Received Invalid ValidatorTx", "error", err.Error(), "tx", etherTx.Tx)
		}
		app.logger.Info("DeliverTx: Received valid ValidatorTx", "tx", etherTx.Tx) // nolint: errcheck
		return abciTypes.ResponseDeliverTx{
			Code: abciTypes.CodeTypeOK,
		}
	case lkTypes.LKTxType:
		lkx := &lkTypes.LKTransaction{}
		err := lkx.Decode(etherTx.Tx)
		if err != nil {
			app.logger.Debug("DeliverTx: Decode LKTransaction", "err", err, "tx", etherTx.Tx)
			return abciTypes.ResponseDeliverTx{
				Code: 1,
				Log:  "Decode LKTransaction error",
			}
		}
		app.logger.Debug("DeliverTx: Received LKTransaction", "type", lkx.Type)
		if lkx.Type != lkTypes.EthTxType {
			app.logger.Info("DeliverTx: invalid tx type", "type", lkx.Type)
			return abciTypes.ResponseDeliverTx{
				Code: 1,
				Log:  "invalid tx type",
			}
		}
		if err = app.handleLKTransaction(lkx); err != nil {
			return abciTypes.ResponseDeliverTx{
				Code: 1,
				Log:  err.Error(),
			}
		}
		return abciTypes.ResponseDeliverTx{
			Code: abciTypes.CodeTypeOK,
		}
	default:
		app.logger.Error("CheckTx: Received Invalid Tx", "type", etherTx.Type, "etherTx", string(jsonBytes))
		return abciTypes.ResponseDeliverTx{
			Code: 1,
			Log:  "Unknown Type",
		}
	}
}

// BeginBlock starts a new Ethereum block
// #stable - 0.4.0
func (app *EthermintApplication) BeginBlock(req abciTypes.RequestBeginBlock) abciTypes.ResponseBeginBlock {
	app.logger.Info("BeginBlock") // nolint: errcheck

	// update the eth header with the tendermint header
	app.backend.UpdateHeader(req.Header)

	return abciTypes.ResponseBeginBlock{}
}

// EndBlock accumulates rewards for the validators and updates them
// #stable - 0.4.0
func (app *EthermintApplication) EndBlock(req abciTypes.RequestEndBlock) abciTypes.ResponseEndBlock {
	app.logger.Info("EndBlock", "height", req.Height)
	app.timeMsg <- "continueBlock"
	return abciTypes.ResponseEndBlock{ValidatorUpdates: app.strategy.GetUpdateValidators()}
}

// Commit commits the block and returns a hash of the current state
// #stable - 0.4.0
func (app *EthermintApplication) Commit() abciTypes.ResponseCommit {
	blockHash, err := app.backend.Commit()
	app.logger.Debug("Commit", "blockHash", blockHash) // nolint: errcheck
	if err != nil {
		app.logger.Error("Error getting latest ethereum state", "err", err) // nolint: errcheck
		return abciTypes.ResponseCommit{
			Code: 1,
			Log:  err.Error(),
		}
	}
	state, err := app.getCurrentState()
	if err != nil {
		app.logger.Error("Error getting latest state", "err", err) // nolint: errcheck
		return abciTypes.ResponseCommit{
			Code: 1,
			Log:  err.Error(),
		}
	}
	log.Info("[lk-events] sendcommitblockevent: ", "blockHash", blockHash)
	go events.GetEventSystem().SendCommitBlockEvent(events.CommitBlockEvent{blockHash})
	app.checkTxState = state.Copy()

	return abciTypes.ResponseCommit{Code: 0, Data: blockHash.Bytes()}
}

//-------------------------------------------------------

// validateTx checks the validity of a tx against the blockchain's current state.
// it duplicates the logic in ethereum's tx_pool
func (app *EthermintApplication) validateEthTx(tx *ethTypes.Transaction, txType string) abciTypes.ResponseCheckTx {
	var reportData reporttrans.TransactionReportData
	if tx == nil {
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  "empty tx",
		}
	}

	txHashStr := tx.Hash().String()
	if app.backend.IsEtxExist(tx.Hash()) {
		log.Trace("[lktx] validateEthTx: Discarding already known transaction", "hash", txHashStr)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  fmt.Sprintf("known transaction, hash=%s", txHashStr),
		}
	}

	var signer ethTypes.Signer = ethTypes.FrontierSigner{}
	if tx.Protected() {
		signer = ethTypes.NewEIP155Signer(tx.ChainId())
	}
	// Make sure the transaction is signed properly
	hashcode := false
	from, err := ethTypes.Sender(signer, tx)
	if err != nil {
		log.Error("[lktx] validateEthTx:", "hash", txHashStr, "err", core.ErrInvalidSender)
		reportData.Result = reporttrans.ERR_SENDER_INVALID
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  core.ErrInvalidSender.Error(),
		}
	}

	if tx.To() != nil {
		reportData.To = tx.To().String()
		hashcode = app.checkTxState.GetCodeSize(*tx.To()) > 0
	}
	reportData.From  = from.Hex()
	reportData.Hash  = tx.Hash().String()
	reportData.Cost  = tx.Cost().String()
	reportData.Gas   = tx.Gas().String()
	reportData.Nonce = tx.Nonce()

	lastBlockTime := app.backend.Ethereum().BlockChain().CurrentBlock().Time().Uint64()
	feeUpdateTime := app.backend.Config().FeeUpdateTime
	var gasFail = false
	if feeUpdateTime != 0 && lastBlockTime >= feeUpdateTime {
		gasFail = tx.IllegalGasLimitOrGasPrice(hashcode, true)
	} else {
		gasFail = tx.IllegalGasLimitOrGasPrice(hashcode, false)
	}

	if gasFail {
		log.Error("[ottx] validateEthTx:", "hash", txHashStr, "err", core.ErrGasLimitOrGasPrice)
		reportData.Result = reporttrans.ERR_GAS_LIMIT_OR_GAS_PRICE
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  core.ErrGasLimitOrGasPrice.Error(),
		}
	}

	// Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	if tx.Size() > maxTransactionSize {
		log.Error("[lktx] validateEthTx:", "hash", txHashStr, "err", core.ErrOversizedData)
		reportData.Result = reporttrans.ERR_HEURISTIC_LIMIT
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  core.ErrOversizedData.Error(),
		}
	}

	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	if tx.Value().Sign() < 0 {
		log.Error("[lktx] validateEthTx:", "hash", txHashStr, "err", core.ErrNegativeValue)

		reportData.Result = reporttrans.ERR_SIGN
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  core.ErrNegativeValue.Error(),
		}
	}

	// Check the transaction doesn't exceed the current block limit gas.
	//gasLimit := app.backend.GasLimit()
	//if gasLimit.Cmp(tx.Gas()) < 0 {
	//	log.Error("[lktx] validateEthTx:", "hash", txHashStr, "gas", tx.Gas().Uint64(),
	//		"gasLimit", gasLimit.Uint64(), "err", core.ErrGasLimitReached)
	//	reportData.Result = reporttrans.ERR_GAS_LIMIT
	//	reporttrans.Report(reportData)
	//	return abciTypes.ResponseCheckTx{
	//		Code: 1,
	//		Log:  core.ErrGasLimitReached.Error(),
	//	}
	//}

	currentState := app.checkTxState

	// Make sure the account exist - cant send from non-existing account.
	if !currentState.Exist(from) {
		log.Error("[lktx] validateEthTx: from not exist in this chainzone", "hash", txHashStr, "from", tx.From(), "err", core.ErrInvalidSender)

		reportData.Result = reporttrans.ERR_FROM_NOT_EXIST
		reporttrans.Report(reportData)

		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  core.ErrInvalidSender.Error(),
		}
	}
	// Check if nonce is not strictly increasing
	nonce := currentState.GetNonce(from)
	cacheNonce, _ := app.nonceMap[from]
	if nonce < cacheNonce {
		nonce = cacheNonce
	}

	if nonce > tx.Nonce() {
		log.Error("[lktx] validateEthTx:", "hash", txHashStr, "err", "Nonce too low")

		reportData.Result = reporttrans.ERR_NONCE
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  fmt.Sprintf("Nonce is not strictly increasing. Expected: %d; Got: %d", nonce, tx.Nonce()),
		}
	} else if nonce < tx.Nonce() {
		return abciTypes.ResponseCheckTx{
			Code: 9999,
			Log:  fmt.Sprintf("Nonce is not strictly increasing. Expected: %d; Got: %d", nonce, tx.Nonce()),
		}
	}

	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	currentBalance := currentState.GetBalance(from)
	reportData.FromBalance = currentBalance.String()
	if currentBalance.Cmp(tx.Cost()) < 0 {
		log.Error("[lktx] validateEthTx:", "hash", txHashStr, "txCost", tx.Cost(),
			"balance", currentBalance, "err", core.ErrInsufficientFunds)

		reportData.Result = reporttrans.ERR_FROM_BALANCE
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  fmt.Sprintf("Current balance: %s, tx cost: %s", currentBalance, tx.Cost()),
		}
	}

	intrGas := core.IntrinsicGas(tx.Data(), tx.To() == nil, true) // homestead == true
	if tx.Gas().Cmp(intrGas) < 0 {
		log.Error("[lktx] validateEthTx:", "hash", txHashStr, "intrGas", intrGas,
			"txGas", tx.Gas(), "err", core.ErrIntrinsicGas)

		reportData.Result = reporttrans.ERR_GAS_CMP
		reporttrans.Report(reportData)
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  core.ErrIntrinsicGas.Error(),
		}
	}

	// Update ether balances
	// amount + gasprice * gaslimit
	currentState.SubBalance(from, tx.Cost())
	// tx.To() returns a pointer to a common address. It returns nil
	// if it is a contract creation transaction.
	if to := tx.To(); to != nil {
		currentState.AddBalance(*to, tx.Value())
	}
	currentState.SetNonce(from, tx.Nonce()+1)

	reportData.Result = reporttrans.ERR_SUCC
	reporttrans.Report(reportData)
	reporttrans.UpdateTrans()
	log.Info("[lktx] validateEthTx: success", "hash", txHashStr)

	app.nonceMap[from] = tx.Nonce() + 1

	return abciTypes.ResponseCheckTx{
		Code: 0,
	}
}

func (app *EthermintApplication) checkLKTransaction(lkx *lkTypes.LKTransaction, txType string) abciTypes.ResponseCheckTx {
	// 1. @Todo check signature.(maybe used for ctx && rtx)

	// check signature
	isValid := app.isLKTxSignValid(lkx)
	if !isValid {
		log.Error("[lktx] isLKTxSignValid: invalid", "hash", lkx.Hash())
		return abciTypes.ResponseCheckTx{
			Code: 1,
			Log:  fmt.Sprintf("isLKTxSignValid: invalid, hash=%s", lkx.Hash()),
		}
	}

	if lkx.Type == lkTypes.EthTxType {
		// check history
		hash := lkx.ETx.Hash()
		d, err := lkdb.GetEthTransactionGas(hash.String())
		if err != nil {
			app.logger.Info("[lktx] checkLKTransaction: GetEthTransactionGas", "err", err)
			return abciTypes.ResponseCheckTx{
				Code: 1,
				Log:  fmt.Sprintf("ldb error: %s", err),
			}
		}
		if d != nil {
			return abciTypes.ResponseCheckTx{
				Code: 1,
				Log:  "tx exists",
			}
		}
		return app.validateEthTx(lkx.ETx.Tx, txType)
	}

	return abciTypes.ResponseCheckTx{ Code: 0 }
}

func (app *EthermintApplication) handleLKTransaction(lkx *lkTypes.LKTransaction) error {
	// 1. @Todo check signature.(maybe used for ctx && rtx)
	isValid := app.isLKTxSignValid(lkx)
	if !isValid {
		log.Error("[lktx] isLKTxSignValid: invalid", "hash", lkx.Hash())
		return fmt.Errorf("isLKTxSignValid: invalid, hash=%s", lkx.Hash())
	}

	// 2. handle it
	var err error
	if lkx.Type == lkTypes.EthTxType {
		err = app.backend.DeliverTx(lkx.ETx.Tx)
	}

	if err != nil {
		app.logger.Info("handleLKTransaction:", "type", lkx.Type, "hash", lkx.Hash().String(), "err", err)
		return err
	}
	app.logger.Debug("handleLKTransaction ok: ", "type", lkx.Type, "hash", lkx.Hash().String())

	// 3. @Todo send event to ws-service
	// @Note no need to do it here. we send event to ws-service at Commit phase.

	return nil
}

func (app *EthermintApplication) isLKTxSignValid(tx *lkTypes.LKTransaction) bool {
	if tx == nil {
		return false
	}
	//@TODO: fake signature checker, should be removed in the future
	if len(tx.Sign) == 0 {
		return true
	}

	var pubKey string
	var err error

	switch tx.Type {
	case lkTypes.EthTxType:
		if tx.ETx == nil || tx.ETx.Tx == nil {
			return false
		}
	default:
		return false
	}

	err = lkTypes.CheckTxSign(tx, pubKey)
	if err != nil {
		log.Warn("[lktx] CheckTxSign: failed", "hash", tx.Hash(), "type", tx.Type,  "sign", tx.Sign, "err", err)
		return false
	}
	log.Info("[lktx] CheckTxSign: valid", "hash", tx.Hash(), "type", tx.Type)

	return true
}
