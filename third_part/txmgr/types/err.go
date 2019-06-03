package types

import "errors"

var (
	ErrNotExist           = errors.New("Not Exist")
	ErrInvalidSignature   = errors.New("Invalid Signature")
	ErrInvalidTx          = errors.New("Invalid Tx")
	ErrUnknownEtherTxType = errors.New("Unknown Ether Tx Type")
)
