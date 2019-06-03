package validatorStrategies

import (
	"fmt"
	"errors"
	"bytes"
	"encoding/hex"

	abci "github.com/tendermint/abci/types"

	txValidator "third_part/txEncrypt"
)

// TxBasedValidatorsStrategy represents a strategy to reward validators with CETH
type TxBasedValidatorsStrategy struct {
	currentValidators []*abci.Validator
	updateValidators  []*abci.Validator
}

func NewValidatorsStrategy() *TxBasedValidatorsStrategy {
	return &TxBasedValidatorsStrategy{
		currentValidators: []*abci.Validator{},
		updateValidators:  []*abci.Validator{},
	}
}

// SetValidators updates the current validators
func (strategy *TxBasedValidatorsStrategy) SetValidators(validators []*abci.Validator) {
	strategy.currentValidators = validators
}

// CollectTx collects the rewards for a transaction
func (strategy *TxBasedValidatorsStrategy) CollectTx(tx []byte) error {
	data, err := strategy.CheckValidatorTx(tx)
	if err != nil {
		return err
	}
	strategy.updateValidators = []*abci.Validator{}
	//update Validator
	for _, tx := range data.Txs {
		keyBytes, err := hex.DecodeString(tx.PubKey)
		if err != nil {
			return err
		}
		strategy.updateValidators = append(strategy.updateValidators, &abci.Validator{PubKey: keyBytes, Power: tx.Power})
	}
	newValidators, err := updateValidators(strategy.currentValidators, strategy.updateValidators)
	if err != nil {
		return err
	}
	strategy.currentValidators = newValidators
	return nil
}

// GetCurrentValidators returns the current validators
func (strategy *TxBasedValidatorsStrategy) GetCurrentValidators() []*abci.Validator {
	return strategy.currentValidators
}

// GetUpdateValidators returns the current validators
func (strategy *TxBasedValidatorsStrategy) GetUpdateValidators() []*abci.Validator {
	updates := strategy.updateValidators
	strategy.updateValidators = []*abci.Validator{}
	return updates
}

func (strategy *TxBasedValidatorsStrategy) CheckValidatorTx(tx []byte) (*txValidator.ValidatorTx, error) {
	keyMap := map[string]int{}
	for i := range strategy.currentValidators {
		keyMap[hex.EncodeToString(strategy.currentValidators[i].PubKey)] = int(strategy.currentValidators[i].Power)
	}
	txData, err := txValidator.ParseValidatorTx(string(tx), keyMap)
	if err != nil {
		return nil, fmt.Errorf("%s keys:%v json:%s", err.Error(), keyMap, string(txValidator.FromHex(string(tx))))
	}
	return txData, nil
}

func updateValidators(currentSet []*abci.Validator, updates []*abci.Validator) ([]*abci.Validator, error) {
	// If more or equal than 1/3 of total voting power changed in one block, then
	// a light client could never prove the transition externally. See
	// ./lite/doc.go for details on how a light client tracks validators.
	vp23, err := changeInVotingPowerMoreOrEqualToOneThird(currentSet, updates)
	if err != nil {
		return nil, err
	}
	if vp23 {
		return nil, errors.New("the change in voting power must be strictly less than 1/3")
	}

	for _, v := range updates {
		if v.Power < 0 {
			return nil, fmt.Errorf("Power (%d) overflows int64", v.Power)
		}

		idx, val := getValidatorByPubKey(currentSet, v.PubKey)
		if val == nil {
			// add val
			currentSet = append(currentSet, v)
		} else if v.Power == 0 {
			// remove val
			currentSet = append(currentSet[:idx], currentSet[idx+1:]...)
		} else {
			val.Power = v.Power
		}
	}
	return currentSet, nil
}

func changeInVotingPowerMoreOrEqualToOneThird(currentSet []*abci.Validator, updates []*abci.Validator) (bool, error) {
	threshold := totalVotingPower(currentSet) * 1 / 3
	acc := int64(0)

	for _, v := range updates {
		if v.Power < 0 {
			return false, fmt.Errorf("Power (%d) overflows int64", v.Power)
		}

		_, val := getValidatorByPubKey(currentSet, v.PubKey)
		if val == nil {
			acc += v.Power
		} else {
			np := val.Power - v.Power
			if np < 0 {
				np = -np
			}
			acc += np
		}

		if acc >= threshold {
			return true, nil
		}
	}

	return false, nil
}

func totalVotingPower(currentSet []*abci.Validator) int64 {
	total := int64(0)
	for i := range currentSet {
		total += currentSet[i].Power
	}
	return total
}

func getValidatorByPubKey(currentSet []*abci.Validator, pubKey []byte) (int, *abci.Validator) {
	for i := range currentSet {
		if bytes.Compare(currentSet[i].PubKey, pubKey) == 0 {
			return i, currentSet[i]
		}
	}
	return -1, nil
}
