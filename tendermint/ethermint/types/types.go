package types

import (
	"github.com/tendermint/abci/types"

	txValidator "third_part/txEncrypt"
)

// ValidatorsStrategy is a validator strategy
type ValidatorsStrategy interface {
	SetValidators(validators []*types.Validator)
	CollectTx(tx []byte) error
	GetCurrentValidators() []*types.Validator
	GetUpdateValidators() []*types.Validator
	CheckValidatorTx(tx []byte) (*txValidator.ValidatorTx, error)
}

// Strategy encompasses all available strategies
type Strategy struct {
	ValidatorsStrategy
}
