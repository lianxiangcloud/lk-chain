package validatorStrategies

import (
	abci "github.com/tendermint/abci/types"
	"testing"
	"github.com/tendermint/go-wire"
)

func TestChangeInVotingPowerMoreOrEqualToOneThird(t *testing.T) {
	current := []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("48B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	update := []*abci.Validator{
		{PubKey: wire.BinaryBytes("45B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("44B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	yes, err := changeInVotingPowerMoreOrEqualToOneThird(current, update)
	//over 1/3
	if !yes {
		t.Fatal("over 1/3 not invalid ")
	}

	update = []*abci.Validator{
		{PubKey: wire.BinaryBytes("45B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}

	yes, err = changeInVotingPowerMoreOrEqualToOneThird(current, update)
	// not over 1/3 and no error
	if yes || err != nil {
		t.Fatal(err)
	}

	update = []*abci.Validator{
		{PubKey: wire.BinaryBytes("45B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("44B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: -1},
	}

	yes, err = changeInVotingPowerMoreOrEqualToOneThird(current, update)
	// not over 1/3 but error
	if yes || err == nil {
		t.Fatal(err)
	}
}

func TestUpdateValidators(t *testing.T) {
	current := []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("48B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	update := []*abci.Validator{
		{PubKey: wire.BinaryBytes("45B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("44B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	newValidators, err := updateValidators(current, update)
	if err == nil {
		t.Fatal("over 1/3 not invalid ", current)
	}

	//test delete
	current = []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("48B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	t.Log(current[:2])
	t.Log(current[4:])
	update = []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 0},
	}
	newValidators, err = updateValidators(current, update)
	if idx, _ := getValidatorByPubKey(newValidators, wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E")); err != nil || idx > -1 {
		t.Fatal("delete failed")
	}

	current = []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("48B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	update = []*abci.Validator{
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 0},
	}
	newValidators, err = updateValidators(current, update)
	if idx, _ := getValidatorByPubKey(newValidators, wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E")); err != nil || idx > -1 {
		t.Fatal("delete failed ", err, "index:", idx)
	}

	//test add
	current = []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("48B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	update = []*abci.Validator{
		{PubKey: wire.BinaryBytes("45B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	newValidators, err = updateValidators(current, update)
	if idx, _ := getValidatorByPubKey(newValidators, wire.BinaryBytes("45B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E")); err != nil || idx < 0 {
		t.Fatal("add failed")
	}

	//test update
	current = []*abci.Validator{
		{PubKey: wire.BinaryBytes("49B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("48B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
		{PubKey: wire.BinaryBytes("46B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 10},
	}
	update = []*abci.Validator{
		{PubKey: wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E"), Power: 5},
	}
	newValidators, err = updateValidators(current, update)
	if idx, val := getValidatorByPubKey(newValidators, wire.BinaryBytes("47B6429027ED28C8F1D8B1ADB48E18C5DD3044D42C8748EBD5FCA8E9AE2B306E")); err != nil || idx < 0 || val.Power != 5 {
		t.Fatal("add failed")
	}

}
