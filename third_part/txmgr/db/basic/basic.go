package basic

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"

	"third_part/txmgr/types"
)

type TxBatch struct {
	items map[string][]byte
	Err   error // record vm error.
}

type EthTransactionGas struct {
	GasLimit *big.Int `json:"gas_limit"`
	GasUsed  *big.Int `json:"gas_used"`
	GasPrice *big.Int `json:"gas_price"`
	BlockNum *big.Int `json:"block_num"`

	// Result: 0(success), others(fail, details come with Errmsg field)
	Ret    int    `json:"ret"`
	Errmsg string `json:"errmsg"`
}

var (
	gBlockNumBig *big.Int
	gBlockNum    string
	gLDB         *LDatabase
	gTxIndex  = 1
	ErrLDBNil = errors.New("LDB nil")
)

// Init NewLDatabase
func Init(file string) error {
	var err error
	if gLDB == nil {
		gLDB, err = NewLDatabase(file, 64, 64)
	}
	return err
}

// Close close LDatabase
func Close() {
	if gLDB != nil {
		gLDB.Close()
		gLDB = nil
	}
}

// CurrentBlockNum get current block num.
func CurrentBlockNum() *big.Int {
	return gBlockNumBig
}

// UpdateBlockNum Update BlockNum for db record after BeginBlock.
func UpdateBlockNum(num *big.Int) {
	// when update, we just reset everything.
	gBlockNumBig = new(big.Int).Set(num)
	gBlockNum = fmt.Sprintf("%s", num.String())
	gTxIndex = 1
}

// GetEthTransactionGas query Gas Info of Ethereum Transaction.
func GetEthTransactionGas(txHash string) (*EthTransactionGas, error) {
	if gLDB == nil {
		return nil, ErrLDBNil
	}

	key := fmt.Sprintf("%s-%s", types.EthTxType, txHash)
	data, err := gLDB.Get([]byte(key))
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	var e EthTransactionGas
	err = json.Unmarshal(data, &e)
	if err != nil {
		return nil, err
	}

	return &e, nil
}

// GetContractTransaction read Contract Transaction
func GetContractTransaction(txHash string) (*types.LKTransaction, error) {
	lkxs, err := GetLKTransaction(types.ContractTxType, txHash)
	if err != nil {
		return nil, err
	}

	if len(lkxs) == 0 {
		return nil, nil
	}

	return lkxs[0], nil
}

// GetSuicideTransaction query SuicideTransaction
func GetSuicideTransaction(txHash string) (*types.LKTransaction, error) {
	lkxs, err := GetLKTransaction(types.SuicideTxType, txHash)
	if err != nil {
		return nil, err
	}

	if len(lkxs) == 0 {
		return nil, nil
	}

	return lkxs[0], nil
}

func BeginBatch() *TxBatch {
	return &TxBatch{
		items: make(map[string][]byte),
	}
}

// Put put lkx to batch, call CommitBatch when we has done.
func (tb *TxBatch) Put(lkx *types.LKTransaction) error {
	return tb.putLKTx(lkx)
}

// Commit commit all the internal transactions.
func (tb *TxBatch) Commit() error {
	if len(tb.items) == 0 {
		return nil
	}

	if gLDB == nil {
		return ErrLDBNil
	}

	batch := gLDB.NewBatch()
	for k, v := range tb.items {
		fmt.Printf(">> put %s : %s\n", string(k), string(v))
		if err := batch.Put([]byte(k), v); err != nil {
			return err
		}
	}

	return batch.Write()
}

func (tb *TxBatch) Cancel() {
	if tb != nil {
		for k := range tb.items {
			delete(tb.items, k)
		}
	}
}

// only used for test.
func (tb *TxBatch) show() int {
	fmt.Printf("batch: %d\n", len(tb.items))
	for k := range tb.items {
		fmt.Printf("\tkey: %s\n", k)
	}
	return len(tb.items)
}

// PutLKTransaction put internal Transaction.
func PutLKTransaction(lkx *types.LKTransaction) error {
	if gLDB == nil {
		return ErrLDBNil
	}

	tb := BeginBatch()
	if err := tb.putLKTx(lkx); err != nil {
		tb.Cancel()
		return err
	}
	return tb.Commit()
}

// GetLKTransaction get internal Transaction.
func GetLKTransaction(txType, txHash string) ([]*types.LKTransaction, error) {
	if gLDB == nil {
		return nil, ErrLDBNil
	}
	kvs := make(map[string][]byte)
	var err error

	switch txType {
	//case types.EthTxType: // not support at here.
	case types.ContractTxType, types.SuicideTxType:
		key := fmt.Sprintf("%s-%s", txType, txHash)
		kvs[key] = nil

	case "*":
		kvs[fmt.Sprintf("%s-%s", types.ContractTxType, txHash)] = nil
		kvs[fmt.Sprintf("%s-%s", types.SuicideTxType, txHash)] = nil

	default:
		return nil, fmt.Errorf("invalid lkx type: %s", txType)
	}

	lkxs := make([]*types.LKTransaction, 0)
	if err = getKVs(kvs); err != nil {
		return lkxs, err
	}

	for _, value := range kvs {
		if len(value) == 0 {
			continue
		}
		var lkx types.LKTransaction
		if err = json.Unmarshal(value, &lkx); err != nil {
			return lkxs, err
		}
		lkxs = append(lkxs, &lkx)
	}

	return lkxs, nil
}

// GetLKTransactionsByBlock get internal Transaction by Block number.
func GetLKTransactionsByBlock(txType string, num *big.Int) ([]*types.LKTransaction, error) {
	if gLDB == nil {
		return nil, ErrLDBNil
	}

	switch txType {
	//case types.EthTxType: // Not Support at here
	case types.ContractTxType, types.SuicideTxType:
	default:
		return nil, fmt.Errorf("invalid lkx type: %s", txType)
	}

	keyPrefix := fmt.Sprintf("%s-%s-", txType, num.String())
	kvs, err := getKVsByPrefix(keyPrefix)
	if err != nil {
		return nil, err
	}

	lkxs := make([]*types.LKTransaction, 0)
	for _, value := range kvs {
		if len(value) == 0 {
			continue
		}
		var lkx types.LKTransaction
		if err = json.Unmarshal(value, &lkx); err != nil {
			return lkxs, err
		}
		lkxs = append(lkxs, &lkx)
	}
	if txType == types.SuicideTxType {
		fmt.Printf("height=%v, txs=%v\n", num, lkxs)
	}
	return lkxs, nil
}

// putLKTx put internal transaction with batch
func (tb *TxBatch) putLKTx(lkx *types.LKTransaction) error {
	switch lkx.Type {
	case types.EthTxType:
		etx := lkx.ETx
		key := fmt.Sprintf("%s-%s", types.EthTxType, etx.Hash().String())
		e := EthTransactionGas{
			GasLimit: etx.Tx.Gas(),
			GasPrice: etx.Tx.GasPrice(),
			GasUsed:  etx.GasUsed,
			BlockNum: gBlockNumBig,
			Ret:      etx.Ret,
			Errmsg:   etx.Errmsg,
		}
		data, _ := json.Marshal(&e)
		tb.items[key] = data
		return nil
	case types.ContractTxType:
		ctx := lkx.CTx
		ctx.BlockNum = gBlockNumBig
		if ctx.Index == 0 {
			ctx.Index = gTxIndex
		}

	case types.SuicideTxType:
		stx := lkx.STx
		stx.BlockNum = gBlockNumBig
		if stx.Index == 0 {
			stx.Index = gTxIndex
		}
	default:
		return fmt.Errorf("invalid lkx type: %s", lkx.Type)
	}

	value, _ := json.Marshal(lkx)
	key := fmt.Sprintf("%s-%s", lkx.Type, lkx.Hash().String())
	tb.items[key] = value
	key = fmt.Sprintf("%s-%s-%d", lkx.Type, gBlockNum, gTxIndex)
	tb.items[key] = value
	gTxIndex++
	return nil
}

func getKVs(kvs map[string][]byte) error {
	if len(kvs) == 0 {
		return nil
	}

	for k := range kvs {
		v, err := gLDB.Get([]byte(k))
		if err != nil {
			if err != leveldb.ErrNotFound {
				return err
			}
			continue
		}
		value := make([]byte, len(v))
		copy(value, v)
		kvs[k] = value
	}

	return nil
}

func getKVsByPrefix(keyPrefix string) (map[string][]byte, error) {
	kvs := make(map[string][]byte)
	ldb := gLDB.LDB()
	iter := ldb.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
	defer iter.Release()

	for iter.Next() {
		if !iter.Valid() {
			if err := iter.Error(); err != nil {
				return nil, err
			}
			continue
		}

		value := make([]byte, len(iter.Value()))
		copy(value, iter.Value())

		kvs[string(iter.Key())] = value
	}

	return kvs, nil
}

/*
func putKVs(batch Batch, kvs map[string][]byte, commit bool) error {
	if len(kvs) == 0 {
		return nil
	}

	for k, v := range kvs {
		fmt.Printf("put key: %s\n", string(k))
		if err := batch.Put([]byte(k), v); err != nil {
			return err
		}
	}

	if commit {
		if err := batch.Write(); err != nil {
			return err
		}
	}

	return nil
}
*/
