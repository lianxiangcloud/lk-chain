package main

import (
	"fmt"
	"path/filepath"

	ethUtils "github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	emtUtils "github.com/tendermint/ethermint/cmd/utils"
	"gopkg.in/urfave/cli.v1"
)

var (
	currentHeader    *types.Header
	currentFastBlock *types.Block
	currentBlock     *types.Block
)

// rollback
func rollbackCmd(ctx *cli.Context) error {
	var rollbackBlockNum uint64
	if ctx.GlobalIsSet(emtUtils.LKRollbackBlockByNumberFlag.Name) {
		rollbackBlockNum = ctx.GlobalUint64(emtUtils.LKRollbackBlockByNumberFlag.Name)
	}

	if rollbackBlockNum <= 0 {
		return nil
	}

	ethermintDataDir := emtUtils.MakeDataDir(ctx)

	chainDB, err := ethdb.NewLDBDatabase(filepath.Join(ethermintDataDir, "ethermint/chaindata"), 0, 0)
	if err != nil {
		ethUtils.Fatalf("could not open database: %v", err)
	}
	defer chainDB.Close()

	blockHash := core.GetHeadBlockHash(chainDB)
	if blockHash == (common.Hash{}) {
		ethUtils.Fatalf("head block hash not found")
	}
	blockNum := core.GetBlockNumber(chainDB, blockHash)
	if blockNum <= rollbackBlockNum {
		ethUtils.Fatalf("rollback fail, rollback height: %d head height: %d", rollbackBlockNum, blockNum)
	}

	currentBlock = core.GetBlock(chainDB, blockHash, blockNum)
	if currentBlock == nil {
		ethUtils.Fatalf("head block not found")
	}

	currentHeader = currentBlock.Header()
	if headHash := core.GetHeadHeaderHash(chainDB); headHash != (common.Hash{}) {
		blockNum = core.GetBlockNumber(chainDB, headHash)
		if header := core.GetHeader(chainDB, headHash, blockNum); header != nil {
			currentHeader = header
		}
	}

	currentFastBlock = currentBlock
	if fastBlockHash := core.GetHeadFastBlockHash(chainDB); fastBlockHash != (common.Hash{}) {
		blockNum = core.GetBlockNumber(chainDB, fastBlockHash)
		if block := core.GetBlock(chainDB, fastBlockHash, blockNum); block != nil {
			currentFastBlock = block
		}
	}

	for i := blockNum; i > rollbackBlockNum; i-- {
		if err = rollbackBlock(chainDB, i); err != nil {
			ethUtils.Fatalf("rollback fail, height: %d, err: %v", i, err)
		}
	}
	return nil
}

func rollbackBlock(chainDB *ethdb.LDBDatabase, number uint64) error {
	hash := core.GetCanonicalHash(chainDB, number)
	if hash == (common.Hash{}) {
		return nil
	}

	header := core.GetHeader(chainDB, currentHeader.ParentHash, currentHeader.Number.Uint64()-1)
	if header == nil {
		return fmt.Errorf("parent head not found. height: %d", currentHeader.Number.Uint64())
	}
	err := core.WriteHeadHeaderHash(chainDB, header.Hash())
	if err != nil {
		return fmt.Errorf("insert head hash fail. height: %d, err: %v", currentHeader.Number.Uint64()-1, err)
	}
	currentHeader = header

	block := core.GetBlock(chainDB, currentBlock.ParentHash(), currentBlock.NumberU64()-1)
	if block == nil {
		return fmt.Errorf("parent block not found. height: %d", currentBlock.NumberU64())
	}
	err = core.WriteHeadBlockHash(chainDB, block.Hash())
	if err != nil {
		return fmt.Errorf("insert block hash fail. height: %d, err: %v", currentBlock.NumberU64()-1, err)
	}
	currentBlock = block

	fastBlock := core.GetBlock(chainDB, currentFastBlock.ParentHash(), currentFastBlock.NumberU64()-1)
	if fastBlock == nil {
		return fmt.Errorf("parent fast block not found. height: %d", currentFastBlock.NumberU64())
	}
	err = core.WriteHeadFastBlockHash(chainDB, fastBlock.Hash())
	if err != nil {
		return fmt.Errorf("insert fast block hash fail. height: %d, err: %v", currentFastBlock.NumberU64()-1, err)
	}
	currentFastBlock = fastBlock

	core.DeleteBlock(chainDB, hash, number)

	return nil
}
