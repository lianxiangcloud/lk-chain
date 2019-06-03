package consensus

import (
	"bytes"
	"fmt"
	"hash/crc32"

	"github.com/tendermint/tmlibs/log"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	"github.com/tendermint/tendermint/version"
	abci "github.com/tendermint/abci/types"
	cmn "github.com/tendermint/tmlibs/common"
	dbm "github.com/tendermint/tmlibs/db"
	sm "github.com/tendermint/tendermint/state"
)

var crc32c = crc32.MakeTable(crc32.Castagnoli)

//----------------------------------------------
// Recover from failure during block processing
// by handshaking with the app to figure out where
// we were last and using the WAL to recover there

type Handshaker struct {
	stateDB      dbm.DB
	initialState sm.State
	store        types.BlockStore
	logger       log.Logger

	nBlocks int // number of blocks applied to the state
}

func NewHandshaker(stateDB dbm.DB, state sm.State, store types.BlockStore) *Handshaker {
	return &Handshaker{stateDB, state, store, log.NewNopLogger(), 0}
}

func (h *Handshaker) SetLogger(l log.Logger) {
	h.logger = l
}

func (h *Handshaker) NBlocks() int {
	return h.nBlocks
}

// TODO: retry the handshake/replay if it fails ?
func (h *Handshaker) Handshake(proxyApp proxy.AppConns) error {
	// handshake is done via info request on the query conn
	res, err := proxyApp.Query().InfoSync(abci.RequestInfo{version.Version})
	if err != nil {
		return fmt.Errorf("Error calling Info: %v", err)
	}

	blockHeight := int64(res.LastBlockHeight)
	if blockHeight < 0 {
		return fmt.Errorf("Got a negative last block height (%d) from the app", blockHeight)
	}
	appHash := res.LastBlockAppHash

	h.logger.Info("ABCI Handshake", "appHeight", blockHeight, "appHash", fmt.Sprintf("%X", appHash))

	// TODO: check version

	// force handle blockheight of app
	if h.initialState.LastValidators.Size() == 0 && h.initialState.LastBlockHeight > 0 && h.initialState.LastBlockHeight == blockHeight {
		blockHeight = 0
	}

	if blockHeight > 0 {
		validators := types.TM2PB.Validators(h.initialState.Validators)
		if _, err := proxyApp.Consensus().InitChainSync(abci.RequestInitChain{validators}); err != nil {
			return err
		}
	}
	// replay blocks up to the latest in the blockstore
	_, err = h.ReplayBlocks(h.initialState, appHash, blockHeight, proxyApp)
	if err != nil {
		return fmt.Errorf("Error on replay: %v", err)
	}

	h.logger.Info("Completed ABCI Handshake - Tendermint and App are synced", "appHeight", blockHeight, "appHash", fmt.Sprintf("%X", appHash))
	// TODO: (on restart) replay mempool

	return nil
}

// Replay all blocks since appBlockHeight and ensure the result matches the current state.
// Returns the final AppHash or an error
func (h *Handshaker) ReplayBlocks(state sm.State, appHash []byte, appBlockHeight int64, proxyApp proxy.AppConns) ([]byte, error) {

	storeBlockHeight := h.store.Height()
	stateBlockHeight := state.LastBlockHeight
	h.logger.Info("ABCI Replay Blocks", "appHeight", appBlockHeight, "storeHeight", storeBlockHeight, "stateHeight", stateBlockHeight)

	// If appBlockHeight == 0 it means that we are at genesis and hence should send InitChain
	if appBlockHeight == 0 {
		validators := types.TM2PB.Validators(state.Validators)
		if _, err := proxyApp.Consensus().InitChainSync(abci.RequestInitChain{validators}); err != nil {
			return nil, err
		}

		if storeBlockHeight > 0 {
			return []byte(""), nil
		}
	}

	// First handle edge cases and constraints on the storeBlockHeight
	if storeBlockHeight == 0 {
		return appHash, checkAppHash(state, appHash)

	} else if storeBlockHeight < appBlockHeight {
		// the app should never be ahead of the store (but this is under app's control)
		return appHash, sm.ErrAppBlockHeightTooHigh{storeBlockHeight, appBlockHeight}

	} else if storeBlockHeight < stateBlockHeight {
		// the state should never be ahead of the store (this is under tendermint's control)
		cmn.PanicSanity(cmn.Fmt("StateBlockHeight (%d) > StoreBlockHeight (%d)", stateBlockHeight, storeBlockHeight))

	} else if storeBlockHeight > stateBlockHeight+1 {
		// store should be at most one ahead of the state (this is under tendermint's control)
		cmn.PanicSanity(cmn.Fmt("StoreBlockHeight (%d) > StateBlockHeight + 1 (%d)", storeBlockHeight, stateBlockHeight+1))
	}

	var err error
	// Now either store is equal to state, or one ahead.
	// For each, consider all cases of where the app could be, given app <= store
	if storeBlockHeight == stateBlockHeight {
		// Tendermint ran Commit and saved the state.
		// Either the app is asking for replay, or we're all synced up.
		if appBlockHeight < storeBlockHeight {
			// the app is behind, so replay blocks, but no need to go through WAL (state is already synced to store)
			return h.replayBlocks(state, proxyApp, appBlockHeight, storeBlockHeight, false)

		} else if appBlockHeight == storeBlockHeight {
			// We're good!
			return appHash, checkAppHash(state, appHash)
		}

	} else if storeBlockHeight == stateBlockHeight+1 {
		// We saved the block in the store but haven't updated the state,
		// so we'll need to replay a block using the WAL.
		if appBlockHeight < stateBlockHeight {
			// the app is further behind than it should be, so replay blocks
			// but leave the last block to go through the WAL
			return h.replayBlocks(state, proxyApp, appBlockHeight, storeBlockHeight, true)

		} else if appBlockHeight == stateBlockHeight {
			// We haven't run Commit (both the state and app are one block behind),
			// so replayBlock with the real app.
			// NOTE: We could instead use the cs.WAL on cs.Start,
			// but we'd have to allow the WAL to replay a block that wrote it's ENDHEIGHT
			h.logger.Info("Replay last block using real app")
			state, err = h.replayBlock(state, storeBlockHeight, proxyApp.Consensus())
			return state.AppHash, err

		} else if appBlockHeight == storeBlockHeight {
			// We ran Commit, but didn't save the state, so replayBlock with mock app
			abciResponses, err := sm.LoadABCIResponses(h.stateDB, storeBlockHeight)
			if err != nil {
				return nil, err
			}
			mockApp := newMockProxyApp(appHash, abciResponses)
			h.logger.Info("Replay last block using mock app")
			state, err = h.replayBlock(state, storeBlockHeight, mockApp)
			return state.AppHash, err
		}

	}

	cmn.PanicSanity("Should never happen")
	return nil, nil
}

func (h *Handshaker) replayBlocks(state sm.State, proxyApp proxy.AppConns, appBlockHeight, storeBlockHeight int64, mutateState bool) ([]byte, error) {
	// App is further behind than it should be, so we need to replay blocks.
	// We replay all blocks from appBlockHeight+1.
	//
	// Note that we don't have an old version of the state,
	// so we by-pass state validation/mutation using sm.ExecCommitBlock.
	// This also means we won't be saving validator sets if they change during this period.
	// TODO: Load the historical information to fix this and just use state.ApplyBlock
	//
	// If mutateState == true, the final block is replayed with h.replayBlock()

	var appHash []byte
	var err error
	finalBlock := storeBlockHeight
	if mutateState {
		finalBlock -= 1
	}
	for i := appBlockHeight + 1; i <= finalBlock; i++ {
		h.logger.Info("Applying block", "height", i)
		block := h.store.LoadBlock(i)
		appHash, err = sm.ExecCommitBlock(proxyApp.Consensus(), block, h.logger)
		if err != nil {
			return nil, err
		}

		h.nBlocks += 1
	}

	if mutateState {
		// sync the final block
		state, err = h.replayBlock(state, storeBlockHeight, proxyApp.Consensus())
		if err != nil {
			return nil, err
		}
		appHash = state.AppHash
	}

	return appHash, checkAppHash(state, appHash)
}

// ApplyBlock on the proxyApp with the last block.
func (h *Handshaker) replayBlock(state sm.State, height int64, proxyApp proxy.AppConnConsensus) (sm.State, error) {
	block := h.store.LoadBlock(height)
	meta := h.store.LoadBlockMeta(height)

	blockExec := sm.NewBlockExecutor(h.stateDB, h.logger, proxyApp, types.MockMempool{}, types.MockEvidencePool{})

	var err error
	state, err = blockExec.ApplyBlock(state, meta.BlockID, block)
	if err != nil {
		return sm.State{}, err
	}

	h.nBlocks += 1

	return state, nil
}

func checkAppHash(state sm.State, appHash []byte) error {
	if !bytes.Equal(state.AppHash, appHash) {
		panic(fmt.Errorf("Tendermint state.AppHash does not match AppHash after replay. Got %X, expected %X", appHash, state.AppHash).Error())
	}
	return nil
}

//--------------------------------------------------------------------------------
// mockProxyApp uses ABCIResponses to give the right results
// Useful because we don't want to call Commit() twice for the same block on the real app.

func newMockProxyApp(appHash []byte, abciResponses *sm.ABCIResponses) proxy.AppConnConsensus {
	clientCreator := proxy.NewLocalClientCreator(&mockProxyApp{
		appHash:       appHash,
		abciResponses: abciResponses,
	})
	cli, _ := clientCreator.NewABCIClient()
	err := cli.Start()
	if err != nil {
		panic(err)
	}
	return proxy.NewAppConnConsensus(cli)
}

type mockProxyApp struct {
	abci.BaseApplication

	appHash       []byte
	txCount       int
	abciResponses *sm.ABCIResponses
}

func (mock *mockProxyApp) DeliverTx(tx []byte) abci.ResponseDeliverTx {
	r := mock.abciResponses.DeliverTx[mock.txCount]
	mock.txCount += 1
	return *r
}

func (mock *mockProxyApp) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
	mock.txCount = 0
	return *mock.abciResponses.EndBlock
}

func (mock *mockProxyApp) Commit() abci.ResponseCommit {
	return abci.ResponseCommit{Code: abci.CodeTypeOK, Data: mock.appHash}
}
