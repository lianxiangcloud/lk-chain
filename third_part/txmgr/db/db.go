package db

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/ethdb"

	dbWraper "third_part/txmgr/db/basic"
	"third_part/txmgr/types"
)

// Init NewLDBDatabase
func Init(file string) error {
	return dbWraper.Init(file)
}

// Close close LDBDatabase
func Close() {
	dbWraper.Close()
}

// UpdateBlockNum Update BlockNum for db record after BeginBlock.
func UpdateBlockNum(num *big.Int) {
	dbWraper.UpdateBlockNum(num)
}

// GetEthTransaction read Ethereum Transaction
func GetEthTransaction(edb ethdb.Database, txHash string) (*types.LKTransaction, error) {
	gasInfo, err := dbWraper.GetEthTransactionGas(txHash)
	if err != nil {
		return nil, err
	}
	if gasInfo == nil {
		return nil, types.ErrNotExist
	}

	// get Ethereum Transaction from origin ldb.
	tx, _, _, _ := core.GetTransaction(edb, common.HexToHash(txHash))
	if tx == nil {
		return nil, types.ErrNotExist
	}

	etx := types.EthTransaction{
		GasUsed: gasInfo.GasUsed,
		Tx:      tx,
		Ret:     gasInfo.Ret,
		Errmsg:  gasInfo.Errmsg,
	}

	lkTx := types.LKTransaction{
		Type: types.EthTxType,
		ETx:  &etx,
	}
	return &lkTx, nil
}

// GetEthTransactionGas query Gas Info of Ethereum Transaction.
func GetEthTransactionGas(txHash string) (*dbWraper.EthTransactionGas, error) {
	return dbWraper.GetEthTransactionGas(txHash)
}

// PutEthTransaction write Ethereum Transaction
func PutEthTransaction(etx *types.EthTransaction) error {
	return dbWraper.PutLKTransaction(&types.LKTransaction{
		Type: types.EthTxType,
		ETx:  etx,
	})
}

// GetContractTransaction read Contract Transaction
func GetContractTransaction(txHash string) (*types.LKTransaction, error) {
	return dbWraper.GetContractTransaction(txHash)
}

// GetContractTransactionsByBlock Get Contract Contraction by BlockNum
func GetContractTransactionsByBlock(num *big.Int) ([]*types.LKTransaction, error) {
	return dbWraper.GetLKTransactionsByBlock(types.ContractTxType, num)
}

// GetSuicideTransaction get Suicide Transaction
func GetSuicideTransaction(txHash string) (*types.LKTransaction, error) {
	return dbWraper.GetSuicideTransaction(txHash)
}

// GetSuicideTransactionByBlock get Suicide Transaction by block number
func GetSuicideTransactionByBlock(num *big.Int) ([]*types.LKTransaction, error) {
	return dbWraper.GetLKTransactionsByBlock(types.SuicideTxType, num)
}

// PutLKTransaction write internal Transaction.
// @Todo To Be Continued....
func PutLKTransaction(writer ethdb.Database, tx *types.LKTransaction) error {

	batch := writer.NewBatch()
	var value []byte
	switch tx.Type {
	case types.EthTxType:
	case types.ContractTxType:
		value, _ = json.Marshal(tx.CTx)
	default:
		return fmt.Errorf("invalid type")
	}

	// TypePrefix-TxHash
	key := fmt.Sprintf("%s-%s", tx.Type, tx.Hash().String())
	if err := batch.Put([]byte(key), value); err != nil {
		return err
	}

	/*
		// TypePrefix-BlockNum-Index
		key = fmt.Sprintf("%s-%s-%d", )
		if err := batch.Put([])
	*/
	return nil
}
