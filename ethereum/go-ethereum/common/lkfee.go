package common

import "math/big"

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
	MaxGasLimit     = 5e9   // max gas limit (500  lianke)
	MinGasLimit     = 5e5	// min gas limit (0.05 lianke)

	everLiankeFee   = 5e4	// ever poundage fee unit(gas)
	gasToLiankeRate = 1e7	// lianke = 1e+7 gas
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