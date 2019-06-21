package common

import (
	"errors"
	"math/big"
)

/*
	手续费为交易金额的千分之五，单笔最低为0.05链克，最高为500链克；在计算时，不足
	1链克的小数部分按照1链克计算，例如一笔交易金额为1000.1链克的交易的手续费为
	5.005链克，1001链克的交易手续费也为5.005链克

	if (x % (1e+18) != 0) {
		fee = (x/(1e+18)+1) * rate *(1e+13)
	} else {
		fee = (x/(1e+18)) * rate *(1e+13)
	}
	if fee < min {
		fee = min
	}
	if max != 0 && fee > max {
		fee = max
	}
*/

const (
	MaxGasLimit = 5e9 // max gas limit (500  lianke)
	MinGasLimit = 5e5 // min gas limit (0.05 lianke)

	everLiankeFee   = 5e4 // ever poundage fee unit(gas)
	gasToLiankeRate = 1e7 // lianke = 1e+7 gas
	gasPrice        = 1e11
)

func CalNewFee(value *big.Int) uint64 {
	var liankeCount *big.Int
	lianke := new(big.Int).Mul(big.NewInt(gasPrice), big.NewInt(gasToLiankeRate))
	if new(big.Int).Mod(value, lianke).Uint64() != 0 {
		liankeCount = new(big.Int).Div(value, lianke)
		liankeCount.Add(liankeCount, big.NewInt(1))
	} else {
		liankeCount = new(big.Int).Div(value, lianke)
	}
	calFeeGas := new(big.Int).Mul(big.NewInt(everLiankeFee), liankeCount)

	if calFeeGas.Cmp(big.NewInt(MinGasLimit)) < 0 {
		calFeeGas.Set(big.NewInt(MinGasLimit))
	}
	if MaxGasLimit != 0 && calFeeGas.Cmp(big.NewInt(MaxGasLimit)) > 0 {
		calFeeGas.Set(big.NewInt(MaxGasLimit))
	}
	return calFeeGas.Uint64()
}

func calAllFee(balance *big.Int, amount *big.Int, min *big.Int, max *big.Int) (am *big.Int, changeAm *big.Int, fee uint64) {
	fee = CalNewFee(amount)

	gasUsed := new(big.Int).Mul(big.NewInt(gasPrice), new(big.Int).SetUint64(fee))
	newBalance := new(big.Int).Add(amount, gasUsed)
	changeAmount := new(big.Int).Sub(balance, newBalance)
	if max.Cmp(min) <= 0 || amount.Cmp(min) == 0 {
		am = amount
		changeAm = changeAmount
		return
	}

	if changeAmount.Sign() == 0 {
		am = amount
		changeAm = changeAmount
		return
	} else if changeAmount.Sign() > 0 {
		// amount is less
		min = new(big.Int).Set(amount)
		fix := new(big.Int).Add(amount, max)
		amount = new(big.Int).Div(fix, big.NewInt(2))

		return calAllFee(balance, amount, min, max)
	} else {
		// amount is more
		max = new(big.Int).Set(amount)
		fix := new(big.Int).Add(amount, min)
		amount = new(big.Int).Div(fix, big.NewInt(2))

		return calAllFee(balance, amount, min, max)
	}
}

func CalSweepBalanceFee(balance *big.Int) (amount *big.Int, changeAmount *big.Int, fee uint64, err error) {
	minGasUsed := new(big.Int).Mul(big.NewInt(gasPrice), big.NewInt(MinGasLimit))
	if balance.Cmp(minGasUsed) <= 0 {
		return big.NewInt(0), big.NewInt(0), 0, errors.New("balance too low")
	}

	amount, changeAmount, fee = calAllFee(balance, new(big.Int).Set(balance), big.NewInt(0), new(big.Int).Set(balance))
	return
}
