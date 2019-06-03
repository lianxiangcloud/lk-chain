package types

import (
	"bytes"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func decodeRawTx(txBytes []byte) (*types.Transaction, error) {
	tx := new(types.Transaction)
	rlpStream := rlp.NewStream(bytes.NewBuffer(txBytes), 0)
	if err := tx.DecodeRLP(rlpStream); err != nil {
		return nil, err
	}
	return tx, nil
}

func decodeRawTxString(tx string) (*types.Transaction, error) {
	txBytes, err := hexutil.Decode(tx)
	if err != nil {
		return nil, err
	}
	return decodeRawTx(txBytes)
}