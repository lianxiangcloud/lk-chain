package state

import (
	"testing"
	"github.com/tendermint/tendermint/node"
	cfg "github.com/tendermint/tendermint/config"
	"fmt"
)

func TestLoadState(t *testing.T) {
	config := cfg.DefaultConfig()
	config.DBPath = "~/Desktop/tendermint"
	stateDB, err := node.DefaultDBProvider(&node.DBContext{"state", config})
	if err != nil {
		t.Errorf("DefaultDBProvider: %v", err)
	}

	state := LoadState(stateDB)
	if state.IsEmpty() {
		t.Errorf("LoadState: %v", err)
	}

	fmt.Printf("%s\n", state)
}