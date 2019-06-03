package utils

import (
	"errors"
	"os"
	"reflect"
	"io/ioutil"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
)

var (
	// chainConfig is the chain parameters to run a node on the LK network.
	chainConfig = &params.ChainConfig{
		ChainId:        big.NewInt(29153),
		HomesteadBlock: big.NewInt(0),
		DAOForkBlock:   nil,
		DAOForkSupport: false,
		EIP150Block:    nil,
		EIP150Hash:     common.Hash{},
		EIP155Block:    big.NewInt(0),
		EIP158Block:    big.NewInt(0),
		ByzantiumBlock: big.NewInt(math.MaxInt64), // Don't enable yet

		Ethash:         new(params.EthashConfig),
	}
)

// defaultGenesisBlob is the JSON representation of the default
// genesis file in $GOPATH/src/github.com/tendermint/ethermint/setup/genesis.json
var defaultGenesisBlob = []byte(`
{
	"config": {
		"chainId": 29153,
		"homesteadBlock": 0,
		"eip155Block": 0,
		"eip158Block": 0
	},
	"nonce": "0x42",
	"timestamp": "0x00",
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"mixhash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"difficulty": "0x40",
	"gasLimit": "0x12A05F200",
	"alloc": {
		"0x7eff122b94897ea5b0e2a9abf47b86337fafebdc": { "balance": "10000000000000000000000000000000000" },
		"0xc6713982649D9284ff56c32655a9ECcCDA78422A": { "balance": "10000000000000000000000000000000000" },
		"0x54fb1c7d0f011dd63b08f85ed7b518ab82028100": { "balance": "10000000000000000000000000000000000" }
	}
}`)

var blankGenesis = new(core.Genesis)

var errBlankGenesis = errors.New("could not parse a valid/non-blank Genesis")

// ParseGenesisOrDefault tries to read the content from provided
// genesisPath. If the path is empty or doesn't exist, it will
// use defaultGenesisBytes as the fallback genesis source. Otherwise,
// it will open that path and if it encounters an error that doesn't
// satisfy os.IsNotExist, it returns that error.
func ParseGenesisOrDefault(genesisPath string) (*core.Genesis, error) {
	var genesisBlob = defaultGenesisBlob[:]
	if len(genesisPath) > 0 {
		blob, err := ioutil.ReadFile(genesisPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(blob) >= 2 { // Expecting atleast "{}"
			genesisBlob = blob
		}
	}

	genesis := new(core.Genesis)
	if err := json.Unmarshal(genesisBlob, genesis); err != nil {
		return nil, err
	}
	if reflect.DeepEqual(blankGenesis, genesis) {
		return nil, errBlankGenesis
	}

	return genesis, nil
}

func GenesisBlockNoAlloc() *core.Genesis {
	return &core.Genesis{
		Config:     chainConfig,
		Nonce:      0x42,
		GasLimit:   5000000000,
		Difficulty: big.NewInt(10000000),
		Alloc:      make(core.GenesisAlloc),
	}
}
