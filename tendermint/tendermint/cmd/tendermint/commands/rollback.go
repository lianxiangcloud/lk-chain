package commands

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	bc "github.com/tendermint/tendermint/blockchain"
	sm "github.com/tendermint/tendermint/state"
	dbm "github.com/tendermint/tmlibs/db"
)

func init() {
	RollbackCmd.Flags().Int64("rollbackblocknumber", config.RollbackBlockNumber, "rollback block number")
}

// RollbackCmd rollback tendermint to the height
var RollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "tendermint rollback to the height",
	Run:   rollback,
}

func rollback(cmd *cobra.Command, args []string) {

	rollbackBlockNumber, err := strconv.ParseInt(cmd.Flags().Lookup("rollbackblocknumber").Value.String(), 10, 64)
	if err != nil || rollbackBlockNumber <= 0 {
		panic(fmt.Errorf("RollbackBlockNumber <= 0, err: %v", err))
	}

	blockStoreDB := dbm.NewDB("blockstore", config.DBBackend, config.DBDir())
	defer blockStoreDB.Close()
	blockStore := bc.NewBlockStore(blockStoreDB)

	stateDB := dbm.NewDB("state", config.DBBackend, config.DBDir())
	defer stateDB.Close()
	state := sm.LoadState(stateDB)

	err = rollbackBlocks(rollbackBlockNumber, blockStore, &state, blockStoreDB, stateDB)
	if err != nil {
		panic(err)
	}
}

func rollbackBlocks(height int64, blockStore *bc.BlockStore, state *sm.State, blockStoreDB, stateDB dbm.DB) error {

	currblocknum := state.LastBlockHeight

	if height >= currblocknum {
		return fmt.Errorf("height >= currblocknum height: %d currblocknum: %d", height, currblocknum)
	}

	targetblock := blockStore.LoadBlock(height + 1)
	if targetblock == nil {
		return fmt.Errorf("the height %d block not found", height+1)
	}

	state.AppHash = targetblock.Header.AppHash
	state.LastBlockID = targetblock.Header.LastBlockID
	state.LastResultsHash = targetblock.Header.LastResultsHash

	state.LastBlockHeight = height
	targetblock = blockStore.LoadBlock(state.LastBlockHeight)
	if targetblock == nil {
		return fmt.Errorf("the height %d block not found", height)
	}

	state.LastBlockTotalTx = targetblock.Header.TotalTxs
	state.LastBlockTime = targetblock.Header.Time

	vals, err := sm.LoadValidators(stateDB, height+1)
	if err != nil {
		return fmt.Errorf("LoadValidators fail. height %d err: %v", height+1, err)
	}
	state.Validators = vals

	state.LastValidators, err = sm.LoadValidators(stateDB, height)
	if err != nil {
		return fmt.Errorf("LoadValidators fail. height %d err: %v", height, err)
	}
	state.ConsensusParams, err = sm.LoadConsensusParams(stateDB, height+1)
	if err != nil {
		return fmt.Errorf("LoadConsensusParams fail. height %d err: %v", height+1, err)
	}

	// save state
	sm.SaveState(stateDB, *state)

	// save BlockStoreStateJSON
	bc.BlockStoreStateJSON{Height: height}.Save(blockStoreDB)

	// delete blocks
	for i := currblocknum; i > height; i-- {
		sm.DeleteABCIResponses(stateDB, i)
		blockStore.DeleteBlock(i)
	}

	return nil
}
