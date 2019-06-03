package basic

import (
	"fmt"
	"math/big"
	"testing"

	lkTypes "third_part/txmgr/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	testBlockNum = big.NewInt(100)
)

func TestUpdateBlockNum(t *testing.T) {
	if gBlockNumBig != nil {
		t.Fatalf("gBlockNumBig should be nil")
	}

	UpdateBlockNum(testBlockNum)
	if gBlockNumBig.Cmp(testBlockNum) != 0 {
		t.Fatalf("gBlockNumBig should be 100")
	}
}

var (
	testEtx = lkTypes.EthTransaction{
		Tx:      types.NewTransaction(1, common.HexToAddress("0x16caeda0f71ece7241b469b88fed39f03a7ec9df"), big.NewInt(10000), big.NewInt(200), big.NewInt(20000), nil),
		GasUsed: big.NewInt(2100),
		Ret:     -1,
		Errmsg:  "invalid operation",
	}

	testCtx = lkTypes.ContractTransaction{
		TxHash: common.HexToHash("0x90599455c3530c5ba085487dc64a34ec057f4740a82f370a52666df785ed2d02"),
		From:   common.HexToAddress("0xea574bd68b9d930aaab4ef9f5c306e67fb26999a"),
		To:     common.HexToAddress("0x16caeda0f71ece7241b469b88fed39f03a7ec9df"),
		Value:  big.NewInt(1000),
	}

	testStx = lkTypes.SuicideTransaction{
		TxHash: common.HexToHash("0x90599455c3530c5ba085487dc64a34ec057f4740a82f370a52666df785ed2d02"),
		From:   common.HexToAddress("0xea574bd68b9d930aaab4ef9f5c306e67fb26999a"),
		To:     common.HexToAddress("0x16caeda0f71ece7241b469b88fed39f03a7ec9df"),
		Target: common.HexToAddress("0x16caeda0f71ece7241b469b88fed39f03a7ec9df"),
		Value:  big.NewInt(1000),
	}
)

func TestPutTransaction(t *testing.T) {
	if err := Init("eth"); err != nil {
		t.Fatalf("Init fail: %s", err)
	}
	defer Close()

	UpdateBlockNum(testBlockNum)

	if err := PutLKTransaction(&lkTypes.LKTransaction{Type: lkTypes.EthTxType, ETx: &testEtx}); err != nil {
		t.Fatalf("PutEthTransaction fail: %s", err)
	}
	t.Logf("testEtx hash: %s", testEtx.Hash().String())

	if err := PutLKTransaction(&lkTypes.LKTransaction{Type: lkTypes.ContractTxType, CTx: &testCtx}); err != nil {
		t.Fatalf("PutContractTransaction fail: %s", err)
	}
	t.Logf("testCtx hash: %s", testCtx.Hash().String())

	if err := PutLKTransaction(&lkTypes.LKTransaction{Type: lkTypes.SuicideTxType, STx: &testStx}); err != nil {
		t.Fatalf("PutSuicideTransaction fail: %s", err)
	}
	t.Logf("testStx hash: %s", testStx.Hash().String())

	t.Logf("PutTransaction done")
}

func TestGetTransaction(t *testing.T) {
	if err := Init("eth"); err != nil {
		t.Fatalf("Init fail: %s", err)
	}
	defer Close()

	etxGas, err := GetEthTransactionGas(testEtx.Hash().String())
	if err != nil {
		t.Fatalf("GetEthTransactionGas fail: %s", err)
	}
	if etxGas == nil {
		t.Fatalf("GetEthTransactionGas not found for tx_hash=%s", testEtx.Hash().String())
	}
	if etxGas.BlockNum.Cmp(testBlockNum) != 0 || etxGas.GasUsed.Cmp(testEtx.GasUsed) != 0 ||
		etxGas.GasLimit.Cmp(testEtx.Tx.Gas()) != 0 || etxGas.Ret != testEtx.Ret || etxGas.Errmsg != testEtx.Errmsg {
		t.Fatalf("invalid gas info: %v", etxGas)
	}
	t.Logf("GetEthTransactionGas ok")

	lkx, err := GetContractTransaction(testCtx.Hash().String())
	if err != nil {
		t.Fatalf("GetContractTransaction fail: %s", err)
	}
	if lkx == nil {
		t.Fatalf("GetContractTransaction not found for hash=%s", testCtx.Hash().String())
	}
	if lkx.CTx.BlockNum.Cmp(testBlockNum) != 0 || lkx.CTx.From != testCtx.From ||
		lkx.CTx.To != testCtx.To || lkx.CTx.Value.Cmp(testCtx.Value) != 0 ||
		lkx.CTx.Index != 2 {
		t.Fatalf("invalid ContractTransaction: %v", lkx.CTx)
	}
	t.Logf("GetContractTransaction ok: %s", lkx.Hash().String())

	lkx, err = GetSuicideTransaction(testStx.Hash().String())
	if err != nil {
		t.Fatalf("GetSuicideTransaction fail: %s", err)
	}
	if lkx == nil {
		t.Fatalf("GetSuicideTransaction not found for hash=%s", testStx.Hash().String())
	}
	if lkx.STx.BlockNum.Cmp(testBlockNum) != 0 || lkx.STx.From != testStx.From ||
		lkx.STx.To != testStx.To || lkx.STx.Value.Cmp(testStx.Value) != 0 ||
		lkx.STx.Index != 3 {
		t.Fatalf("invalid SuicideTransaction: %v", lkx.STx)
	}
	t.Logf("GetSuicideTransaction ok: %s", lkx.Hash().String())
}

func TestGetTransactionByBlockNum(t *testing.T) {
	if err := Init("eth"); err != nil {
		t.Fatalf("Init fail: %s", err)
	}
	defer Close()

	lkxs, err := GetLKTransactionsByBlock(lkTypes.ContractTxType, testBlockNum)
	if err != nil {
		t.Fatalf("GetContractTransactionByBlock fail: %s", err)
	}
	if len(lkxs) != 1 {
		t.Fatalf("GetContractTransactionByBlock not found for block_num=%s", testBlockNum.String())
	}
	lkx = lkxs[0]
	if lkx.CTx.BlockNum.Cmp(testBlockNum) != 0 || lkx.CTx.From != testCtx.From ||
		lkx.CTx.To != testCtx.To || lkx.CTx.Value.Cmp(testCtx.Value) != 0 {
		t.Fatalf("invalid ContractTransaction")
	}
	t.Logf("GetContractTransactionByBlock ok: %s", lkx.Hash().String())

	lkxs, err = GetLKTransactionsByBlock(lkTypes.SuicideTxType, testBlockNum)
	if err != nil {
		t.Fatalf("GetSuicideTransactionByBlock fail: %s", err)
	}
	if len(lkxs) != 1 {
		t.Fatalf("GetSuicideTransactionByBlock not found for block_num=%s", testBlockNum.String())
	}
	lkx = lkxs[0]
	if lkx.STx.BlockNum.Cmp(testBlockNum) != 0 || lkx.STx.From != testStx.From ||
		lkx.STx.To != testStx.To || lkx.STx.Value.Cmp(testStx.Value) != 0 {
		t.Fatalf("invalid SuicideTransaction")
	}
	t.Logf("GetSuicideTransactionByBlock ok: %s", lkx.Hash().String())
}

func TestBatchCancel(t *testing.T) {
	UpdateBlockNum(testBlockNum)
	if err := Init("eth"); err != nil {
		t.Fatalf("Init fail: %s", err)
	}
	defer Close()

	if err := cleardb(); err != nil {
		t.Fatalf("cleardb fail:  %s", err)
	}

	batch := BeginBatch()

	if err := batch.Put(&lkTypes.LKTransaction{Type: lkTypes.EthTxType, ETx: &testEtx}); err != nil {
		t.Fatalf("batch.Put %s fail: %s", lkTypes.EthTxType, err)
	}

	if err := batch.Put(&lkTypes.LKTransaction{Type: lkTypes.ContractTxType, CTx: &testCtx}); err != nil {
		t.Fatalf("batch.Put %s fail: %s", lkTypes.ContractTxType, err)
	}

	if err := batch.Put(&lkTypes.LKTransaction{Type: lkTypes.SuicideTxType, STx: &testStx}); err != nil {
		t.Fatalf("batch.Put %s fail: %s", lkTypes.SuicideTxType, err)
	}

	batch.Cancel()
	if batch.show() != 0 {
		t.Fatalf("batch not empty")
	}
}

func TestBatchCommit(t *testing.T) {
	UpdateBlockNum(testBlockNum)
	if err := Init("eth"); err != nil {
		t.Fatalf("Init fail: %s", err)
	}
	defer Close()

	if err := cleardb(); err != nil {
		t.Fatalf("cleardb fail:  %s", err)
	}

	batch := BeginBatch()

	if err := batch.Put(&lkTypes.LKTransaction{Type: lkTypes.EthTxType, ETx: &testEtx}); err != nil {
		t.Fatalf("batch.Put %s fail: %s", lkTypes.EthTxType, err)
	}

	if err := batch.Put(&lkTypes.LKTransaction{Type: lkTypes.ContractTxType, CTx: &testCtx}); err != nil {
		t.Fatalf("batch.Put %s fail: %s", lkTypes.ContractTxType, err)
	}

	if err := batch.Put(&lkTypes.LKTransaction{Type: lkTypes.SuicideTxType, STx: &testStx}); err != nil {
		t.Fatalf("batch.Put %s fail: %s", lkTypes.SuicideTxType, err)
	}

	if err := batch.Commit(); err != nil {
		t.Fatalf("batch.Commit fail: %s", err)
	}

	etxGas, err := GetEthTransactionGas(testEtx.Hash().String())
	if err != nil {
		t.Fatalf("GetEthTransactionGas fail: %s", err)
	}
	if etxGas == nil {
		t.Fatalf("GetEthTransactionGas not found for tx_hash=%s", testEtx.Hash().String())
	}
	if etxGas.BlockNum.Cmp(testBlockNum) != 0 || etxGas.GasUsed.Cmp(testEtx.GasUsed) != 0 ||
		etxGas.GasLimit.Cmp(testEtx.Tx.Gas()) != 0 || etxGas.Ret != testEtx.Ret || etxGas.Errmsg != testEtx.Errmsg {
		t.Fatalf("invalid gas info: %v", etxGas)
	}
	t.Logf("GetEthTransactionGas ok: %v", etxGas)

	TestGetTransactionByBlockNum(t)

	/*
		keys, err := getAllKeys()
		if err != nil {
			t.Fatalf("getAllKeys fail: %s", err)
		}
		if len(keys) == 0 {
			t.Fatalf("keys still exist")
		}

		fmt.Printf("commit %d keys:\n", len(keys))
		for _, k := range keys {
			fmt.Printf("\tkey: %s\n", string(k))
		}
	*/
}

func TestClearDB(t *testing.T) {
	UpdateBlockNum(testBlockNum)
	if err := Init("eth"); err != nil {
		t.Fatalf("Init fail: %s", err)
	}
	defer Close()

	if err := cleardb(); err != nil {
		t.Fatalf("cleardb fail:  %s", err)
	}

	keys, err := getAllKeys()
	if err != nil {
		t.Fatalf("getAllKeys fail: %s", err)
	}
	if len(keys) != 0 {
		fmt.Printf("keys still: \n")
		for _, k := range keys {
			fmt.Printf("\tkey: %s\n", k)
		}
		t.Fatalf("len(%d) > 0", len(keys))
	}
}

func cleardb() error {
	keys, err := getAllKeys()
	if err != nil {
		return err
	}

	for _, k := range keys {
		if err := gLDB.Delete(k); err != nil {
			fmt.Printf("delete fail: %s\n", string(k))
			return err
		}
		fmt.Printf("delete ok: %s\n", string(k))
	}
	return nil
}

func getAllKeys() ([][]byte, error) {
	if gLDB == nil {
		return nil, ErrLDBNil
	}

	keys := make([][]byte, 0)
	iter := gLDB.NewIterator()
	for iter.Next() {
		keys = append(keys, iter.Key())
	}
	iter.Release()

	return keys, nil
}
