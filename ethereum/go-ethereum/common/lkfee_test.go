package common

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"
)

func TestCalNewFee(t *testing.T) {
	type gasTest struct {
		val    *big.Int
		gasFee uint64
	}

	lianke := new(big.Int).Mul(big.NewInt(gasPrice), big.NewInt(gasToLiankeRate))

	// test msg
	gasTestMsg := []gasTest{
		{big.NewInt(0), MinGasLimit},
		{big.NewInt(10), MinGasLimit},
		{lianke, MinGasLimit},
		{new(big.Int).Mul(big.NewInt(10), lianke), MinGasLimit},
		{new(big.Int).Mul(big.NewInt(100), lianke), 5e6},
		{new(big.Int).Mul(big.NewInt(1e10), lianke), MaxGasLimit},
		{new(big.Int).Mul(big.NewInt(1e11), lianke), MaxGasLimit},
	}

	// check
	for _, v := range gasTestMsg {
		calFee := CalNewFee(v.val)
		if v.gasFee != calFee {
			t.Fatal("CalNewFee failed.", "val", v.val, "gasFee", v.gasFee, "calFee", calFee)
		}
	}
}

func newBig(v string) *big.Int {
	b, _ := new(big.Int).SetString(v, 0)
	return b
}

// /*
func TestCalSweepBalanceFee(t *testing.T) {
	for index := 0; index < 100000; index++ {
		a := rand.Int63n(100000)
		b := rand.Int63()
		min := int64(50000000000000000)
		if b < min {
			b = b + min
		}
		balance := new(big.Int).Add(new(big.Int).Mul(big.NewInt(a), big.NewInt(1e+18)), big.NewInt(b))
		amount, fee, _ := CalSweepBalanceFee(balance)
		calFee := CalNewFee(amount)
		if fee != fee {
			t.Fatal(fmt.Sprintf("CalNewFee != SweepBalanceFee,balance:%s, %d != %d ", balance.String(), calFee, fee))
		}
		b2 := new(big.Int).Add(amount, new(big.Int).Mul(new(big.Int).SetUint64(fee), big.NewInt(gasPrice)))
		diff := new(big.Int).Sub(balance, b2)
		fmt.Printf("%d, balance: %s ,amount: %s ,fee %d ,diff: %s\n", index, balance.String(), amount.String(), fee, diff.String())
		if diff.Cmp(big.NewInt(3e+16)) > 0 {
			t.Fatal("diff more then 0.05")
		}
	}
}

// */
/*
func TestCalSweepBalanceFee(t *testing.T) {
	type TestBalanceFee struct {
		Balance *big.Int
		Amount  *big.Int
		Fee     uint64
		Err     error
	}

	testFeeArr := []TestBalanceFee{
		//       1000000000000000000  ,1 lk
		{newBig("50000000000000000"), newBig("1"), uint64(500000), errors.New("balance too low")},
		{newBig("50000000000000001"), newBig("1"), uint64(500000), nil},
		{newBig("78997651234567899"), newBig("28997651234567899"), uint64(500000), nil},
		{newBig("100012512345678995"), newBig("50012512345678995"), uint64(500000), nil},
		{newBig("500000000000000000"), newBig("450000000000000000"), uint64(500000), nil},
		{newBig("1345125123456789956"), newBig("1295125123456789956"), uint64(500000), nil},
		{newBig("11345125123456789956"), newBig("11285125123456789956"), uint64(600000), nil},
		{newBig("119000000000000000000"), newBig("118405000000000000000"), uint64(5950000), nil},
		{newBig("1190000000000000000000"), newBig("1184075000000000000000"), uint64(59250000), nil},
		{newBig("11900000000000123456789"), newBig("11840795000000123456789"), uint64(59250000), nil},
		{newBig("119000000000000123456789"), newBig("118500000000000123456789"), uint64(5000000000), nil},
		{newBig("1191234567899765123456789"), newBig("1190734567899765123456789"), uint64(5000000000), nil},
	}

	for _, v := range testFeeArr {
		amount, fee, err := CalSweepBalanceFee(v.Balance)
		if v.Err != nil {
			if err != nil {
				fmt.Println(err.Error())
				continue
			} else {
				t.Fatal("SweepBalanceFee failed. balance : ", v.Balance.String())
			}
		}
		if err != nil {
			t.Fatal("SweepBalanceFee failed. balance : ", v.Balance.String())
		}

		diff := new(big.Int).Sub(v.Balance, new(big.Int).Add(amount, new(big.Int).Mul(new(big.Int).SetUint64(fee), big.NewInt(gasPrice))))
		fmt.Printf("balance: %s ,amount: %s ,fee %d ,diff: %d\n", v.Balance.String(), amount.String(), fee, diff.Uint64())
	}
}
*/
