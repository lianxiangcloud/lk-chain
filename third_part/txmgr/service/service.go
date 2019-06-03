package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	etypes "github.com/ethereum/go-ethereum/core/types"

	lkdb "third_part/txmgr/db"
	"third_part/txmgr/events"
	"third_part/txmgr/types"
)

const (
	// commitBlockChanSize is the size of channel listening to CommitBlockEvent.
	commitBlockChanSize = 10
)

type Service struct {
	eth          *eth.Ethereum
	es           *events.EventSystem
	filterEvents *filters.EventSystem
}

func NewService(eth *eth.Ethereum) (*Service, error) {
	if eth == nil || eth.ApiBackend == nil {
		return nil, errors.New("NewService: invalid parameter")
	}

	var service *Service = &Service{
		eth:          eth,
		es:           events.GetEventSystem(),
		filterEvents: filters.NewEventSystem(eth.ApiBackend.EventMux(), eth.ApiBackend, false),
	}
	return service, nil
}

func (s *Service) GetWrappedTransaction(req *types.TxRecordReq) (*types.WrappedLKTransaction, error) {
	lkx, err := s.GetTransaction(req)
	if err != nil {
		log.Error("GetWrappedTransaction: ", "err", err)
		return nil, fmt.Errorf("GetWrappedTransaction: %v", err)
	}
	var wtx = &types.WrappedLKTransaction{
		LKx:      lkx,
		BlockNum: big.NewInt(0),
	}
	switch req.Type {
	case types.EthTxType:
		if tx, _, blockNum, _ := core.GetTransaction(s.eth.ChainDb(), lkx.Hash()); tx != nil {
			wtx.BlockNum = big.NewInt(int64(blockNum))
		}
	case types.ContractTxType:
		wtx.BlockNum = lkx.CTx.BlockNum
	case types.SuicideTxType:
		wtx.BlockNum = lkx.CTx.BlockNum
	default:
		wtx.BlockNum = nil
	}
	return wtx, nil
}

func (s *Service) getTransactionByHash(txType string, hash common.Hash) (*types.LKTransaction, error) {
	if tx, _, _, _ := core.GetTransaction(s.eth.ChainDb(), hash); tx != nil {
		lkTx := &types.LKTransaction{
			Type: txType,
			ETx: &types.EthTransaction{
				Tx:      tx,
				GasUsed: tx.Gas(),
			},
		}
		return lkTx, nil
	}
	//@Note: No finalized transaction, maybe we should try to retrieve it from the pool
	log.Debug("[lkrpc] getTransactionByHash: tx not exist", "hash", hash)
	return nil, types.ErrNotExist
}

// GetTransaction query transaction
func (s *Service) GetTransaction(req *types.TxRecordReq) (*types.LKTransaction, error) {
	var tx *types.LKTransaction
	var err error
	switch req.Type {
	case types.EthTxType:
		tx, err = lkdb.GetEthTransaction(s.eth.ChainDb(), req.TxHash)
		if err != nil {
			log.Warn("[lkrpc] GetEthTransaction: ignore failure", "hash", req.TxHash, "err", err)
		}
		if tx == nil || tx.ETx == nil {
			return s.getTransactionByHash(types.EthTxType, common.HexToHash(req.TxHash))
		}
	case types.ContractTxType:
		tx, err = lkdb.GetContractTransaction(req.TxHash)
		if err != nil {
			return nil, fmt.Errorf("GetContractTransaction: hash=%s, err=%s", req.TxHash, err)
		}
	case types.SuicideTxType:
		tx, err = lkdb.GetSuicideTransaction(req.TxHash)
		if err != nil {
			return nil, fmt.Errorf("GetSuicideTransaction: hash=%s, err=%v", req.TxHash, err)
		}
	}

	if tx == nil {
		return nil, fmt.Errorf("GetTransaction: hash=%s, type=%s, err=%v", req.TxHash, req.Type, types.ErrNotExist)
	}
	return tx, nil
}

// GetBlock query block
func (s *Service) GetBlock(num *big.Int) (*types.LKBlock, error) {
	var lkBlock *types.LKBlock = &types.LKBlock{
		Number: num,
	}

	ethBlock := s.eth.BlockChain().GetBlockByNumber(num.Uint64())
	if ethBlock == nil {
		return nil, fmt.Errorf("GetBlockByNumber: %v", types.ErrNotExist)
	}
	lkBlock.Hash = ethBlock.Hash()
	lkBlock.Time = ethBlock.Time()

	var lkTxs []*types.LKTransaction = make([]*types.LKTransaction, 0)

	for _, tx := range ethBlock.Transactions() {
		txGas, err := lkdb.GetEthTransactionGas(tx.Hash().String())
		if err != nil {
			log.Warn("[lkrpc] GetEthTransactionGas: ignore failure", "blockNumber", num, "err", err)
		}
		lkTx := &types.LKTransaction{
			Type: types.EthTxType,
			ETx: &types.EthTransaction{
				Tx: tx,
			},
		}
		if txGas != nil {
			lkTx.ETx.GasUsed = txGas.GasUsed
			lkTx.ETx.Ret = txGas.Ret
			lkTx.ETx.Errmsg = txGas.Errmsg
		} else {
			log.Info("[lkrpc] GetEthTransactionGas: gas not exist", "txHash", tx.Hash().String(), "blockNum", ethBlock.Number())
		}
		lkTxs = append(lkTxs, lkTx)
	}

	ctxs, err := lkdb.GetContractTransactionsByBlock(lkBlock.Number)
	if err != nil {
		log.Warn("[lkrpc] GetContractTransactionsByBlock: ignore failure", "blockNumber", num, "err", err)
	}
	lkTxs = append(lkTxs, ctxs...)

	stxs, err := lkdb.GetSuicideTransactionByBlock(lkBlock.Number)
	if err != nil {
		return nil, fmt.Errorf("GetSuicideTransactionByBlock: %v", err)
	}
	log.Info("[lkrpc] GetSuicideTransactionsByBlock", "blockNum", ethBlock.Number(), "stxs", stxs)
	lkTxs = append(lkTxs, stxs...)

	lkBlock.Transactions = lkTxs

	return lkBlock, nil
}

// getBlockByHash:
func (s *Service) getBlockByHash(hash common.Hash) (*types.LKBlock, error) {
	var lkBlock *types.LKBlock = &types.LKBlock{}
	ethBlock := s.eth.BlockChain().GetBlockByHash(hash)
	if ethBlock == nil {
		return nil, types.ErrNotExist
	}
	lkBlock.Hash = ethBlock.Hash()
	lkBlock.Time = ethBlock.Time()
	lkBlock.Number = ethBlock.Number()

	var lkTxs []*types.LKTransaction = make([]*types.LKTransaction, 0)

	for _, tx := range ethBlock.Transactions() {
		txGas, err := lkdb.GetEthTransactionGas(tx.Hash().String())
		if err != nil {
			return nil, fmt.Errorf("GetEthTransactionGas: %v", err)
		}

		lkTx := &types.LKTransaction{
			Type: types.EthTxType,
			ETx: &types.EthTransaction{
				Tx: tx,
			},
		}
		if txGas != nil {
			lkTx.ETx.GasUsed = txGas.GasUsed
			lkTx.ETx.Ret = txGas.Ret
			lkTx.ETx.Errmsg = txGas.Errmsg
		} else {
			log.Info("[lkrpc] GetEthTransactionGas: gas not exist", "txHash", tx.Hash().String(), "blockNum", ethBlock.Number())
		}
		lkTxs = append(lkTxs, lkTx)
	}

	ctxs, err := lkdb.GetContractTransactionsByBlock(lkBlock.Number)
	if err != nil {
		return nil, fmt.Errorf("GetContractTransactionsByBlock: %v", err)
	}
	lkTxs = append(lkTxs, ctxs...)

	stxs, err := lkdb.GetSuicideTransactionByBlock(lkBlock.Number)
	if err != nil {
		return nil, fmt.Errorf("GetSuicideTransactionByBlock: %v", err)
	}
	log.Info("[lkrpc] GetSuicideTransactionsByBlock", "blockNum", ethBlock.Number(), "stxs", stxs)
	lkTxs = append(lkTxs, stxs...)

	lkBlock.Transactions = lkTxs
	return lkBlock, nil
}

// GetBlockNum query current block num
func (s *Service) GetBlockNum() (*big.Int, error) {
	num := s.eth.BlockChain().CurrentBlock().NumberU64()
	return new(big.Int).SetUint64(num), nil
}

// GetBlockNumUint64 query current block num
func (s *Service) GetBlockNumUint64() (uint64, error) {
	return s.eth.BlockChain().CurrentBlock().NumberU64(), nil
}

func (s *Service) GetBalance(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (*big.Int, error) {
	state, _, err := s.eth.ApiBackend.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	b := state.GetBalance(address)
	return b, state.Error()
}

func (s *Service) GetTransactionCount(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (uint64, error) {
	if blockNr == rpc.PendingBlockNumber {
		nonce, err := s.eth.ApiBackend.GetPoolNonce(ctx, address)
		return nonce, err
	} else {
		state, _, err := s.eth.ApiBackend.StateAndHeaderByNumber(ctx, blockNr)
		if state == nil || err != nil {
			return 0, err
		}
		nonce := state.GetNonce(address)
		return nonce, state.Error()
	}
	return 0, nil
}

// BlockSubscribe subscribe new block notifi.
func (s *Service) BlockSubscribe(ctx context.Context) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}

	subscription := notifier.CreateSubscription()

	commitBlockCh := make(chan events.CommitBlockEvent, commitBlockChanSize)
	commitBlockSub := s.es.SubscribeCommitBlockEvent(commitBlockCh)
	log.Info("[lkrpc] subsccribeCommitBlockEvent: ", "id", subscription.ID)

	go func() {
		defer commitBlockSub.Unsubscribe()
		for {
			select {
			case err := <-commitBlockSub.Err():
				if err != nil {
					log.Error("[lkrpc] commibBlockSub: failed", "err", err)
				} else {
					log.Info("[lkrpc] commitBlockSub: exit")
				}
				return
			case commitBlock := <-commitBlockCh:
				block, err := s.getBlockByHash(commitBlock.Hash)
				if err != nil {
					log.Error("[lkrpc] getBlockByHash: failed", "err", err)
					continue
				}

				if err := notifier.Notify(subscription.ID, block); err != nil {
					log.Error("[lkrpc] notify: failed", "err", err, "id", subscription.ID, "blockHash", block.Hash, "blockNum", block.Number)
					return
				}

				log.Debug("[lkrpc] notify: success", "id", subscription.ID, "blockHash", block.Hash, "blockNum", block.Number, "block", block.Transactions)

			case <-notifier.Closed():
				log.Info("[lkrpc] BlockSubscribe: unsubscribe", "id", subscription.ID)
				return
			case err := <-subscription.Err():
				if err != nil {
					log.Error("[lkrpc] subscription: ", "id", subscription.ID, "err", err)
				} else {
					log.Info("[lkrpc] subscription: exit", "id", subscription.ID)
				}
				return
			}
		}
	}()

	log.Info("[lkrpc] BlockSubscribe: success", "id", subscription.ID)
	return subscription, nil
}

// BlockSubscribe subscribe new block notifi.
func (s *Service) LogsSubscribe(ctx context.Context, crit filters.FilterCriteria) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}
	var (
		rpcSub      = notifier.CreateSubscription()
		matchedLogs = make(chan []*etypes.Log)
	)

	logsSub, err := s.filterEvents.SubscribeLogs(crit, matchedLogs)
	if err != nil {
		return nil, err
	}
	log.Info("[lkrpc] LogsSubscribe: success", "id", logsSub.ID)
	go func() {

		for {
			select {
			case logs := <-matchedLogs:
				for _, log := range logs {
					notifier.Notify(rpcSub.ID, &log)
				}
			case <-rpcSub.Err(): // client send an unsubscribe request
				logsSub.Unsubscribe()
				return
			case <-notifier.Closed(): // connection dropped
				logsSub.Unsubscribe()
				return
			}
		}
	}()

	return rpcSub, nil
}

func (s *Service) GetLogs(ctx context.Context, crit filters.FilterCriteria) ([]*etypes.Log, error) {
	// Convert the RPC block numbers into internal representations
	if crit.FromBlock == nil {
		crit.FromBlock = big.NewInt(rpc.LatestBlockNumber.Int64())
	}
	if crit.ToBlock == nil {
		crit.ToBlock = big.NewInt(rpc.LatestBlockNumber.Int64())
	}
	// Create and run the filter to get all the logs
	filter := filters.New(s.eth.ApiBackend, crit.FromBlock.Int64(), crit.ToBlock.Int64(), crit.Addresses, crit.Topics)

	logs, err := filter.Logs(ctx)
	if err != nil {
		return nil, err
	}
	return returnLogs(logs), err
}

// GetTransactionReceipt returns the transaction receipt for the given transaction hash.
func (s *Service) GetTransactionReceipt(hash common.Hash) (map[string]interface{}, error) {
	tx, blockHash, blockNumber, index := core.GetTransaction(s.eth.ChainDb(), hash)
	if tx == nil {
		return nil, nil
	}
	receipt, _, _, _ := core.GetReceipt(s.eth.ChainDb(), hash)

	signer := etypes.NewEIP155Signer(tx.ChainId())
	from, _ := etypes.Sender(signer, tx)

	fields := map[string]interface{}{
		"blockHash":         blockHash,
		"blockNumber":       hexutil.Uint64(blockNumber),
		"transactionHash":   hash,
		"transactionIndex":  hexutil.Uint64(index),
		"from":              from,
		"to":                tx.To(),
		"gasUsed":           (*hexutil.Big)(receipt.GasUsed),
		"cumulativeGasUsed": (*hexutil.Big)(receipt.CumulativeGasUsed),
		"contractAddress":   nil,
		"logs":              receipt.Logs,
		"logsBloom":         receipt.Bloom,
		"status":            hexutil.Uint(receipt.Status),
	}

	if receipt.Logs == nil {
		fields["logs"] = [][]*etypes.Log{}
	}

	// If the ContractAddress is 20 0x0 bytes, assume it is not a contract creation
	if receipt.ContractAddress != (common.Address{}) {
		fields["contractAddress"] = receipt.ContractAddress
	}
	return fields, nil
}

// returnLogs is a helper that will return an empty log array in case the given logs array is nil,
// otherwise the given logs array is returned.
func returnLogs(logs []*etypes.Log) []*etypes.Log {
	if logs == nil {
		return []*etypes.Log{}
	}
	return logs
}
