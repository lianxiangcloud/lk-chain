package ethereum

import (
	"fmt"
	"strings"
	"time"
	"bytes"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	rpcClient "github.com/tendermint/tendermint/rpc/lib/client"
	lkTypes "third_part/txmgr/types"
)

//----------------------------------------------------------------------
// Transactions sent via the go-ethereum rpc need to be routed to tendermint

// listen for txs and forward to tendermint
func (b *Backend) txBroadcastLoop() {
	txCh := make(chan core.TxPreEvent, 40000)
	txSub := b.ethereum.TxPool().SubscribeTxPreEvent(txCh)
	defer txSub.Unsubscribe()

	blockCh := make(chan core.ChainHeadEvent, 5000)
	blockSub := b.ethereum.BlockChain().SubscribeChainHeadEvent(blockCh)
	defer blockSub.Unsubscribe()

	waitForServer(b.client)

	for {
		select {
		case ev := <-txCh:
			if ev.Tx != nil {
				b.transformTx(ev.Tx)
			}
		case ev := <-blockCh:
			//log.Info(">> Got BlockEvent")
			b.transformTx(nil)
			if ev.Block != nil {
				txs := ev.Block.Transactions()
				for _, tx := range txs {
					delete(_txhashMap, tx.Hash())
				}
			}
		case err := <-txSub.Err():
			log.Error(">> txSub error", "err", err)
			return
		case err := <-blockSub.Err():
			log.Error(">> blockSub error", "err", err)
			return
		}
	}
}

var (
	_nonceMap  = make(map[common.Address]uint64)
	_txhashMap = make(map[common.Hash]bool)
	//_txMu      = new(sync.Mutex)

	_blockNum = new(big.Int)
	_dbState  *state.StateDB
)

const (
	_maxTxs = 2000
)

func (b *Backend) getNonce(addr common.Address) uint64 {
	bc := b.ethereum.BlockChain()
	if bc.CurrentHeader().Number.Cmp(_blockNum) > 0 || _dbState == nil {
		_blockNum = bc.CurrentHeader().Number
		_dbState, _ = bc.State()
	}
	nonce := _dbState.GetNonce(addr)

	cacheNonce := _nonceMap[addr]
	if nonce < cacheNonce {
		nonce = cacheNonce
	}
	return nonce
}

func (b *Backend) setNonce(addr common.Address, nonce uint64) {
	_nonceMap[addr] = nonce
}

func (b *Backend) transformTx(tx *ethTypes.Transaction) {
	_start := time.Now()
	counts := 0
	total := 0
	var mtxs map[common.Address]ethTypes.Transactions
	if tx == nil {
		time.Sleep(time.Millisecond * 5) // just wait
		mtxs, _ = b.ethereum.TxPool().PendingTxs(_maxTxs)
		log.Debug(">> got PendingTxs", "used", fmt.Sprintf("%dms", time.Since(_start)/time.Millisecond))
	} else {
		mtxs = make(map[common.Address]ethTypes.Transactions)
		addr := common.HexToAddress(tx.From())
		txs := make([]*ethTypes.Transaction, 1)
		txs[0] = tx
		mtxs[addr] = txs
	}

	for addr, txs := range mtxs {
		if txs.Len() == 0 {
			continue
		}
		total += txs.Len()

		nonce := b.getNonce(addr)
		for _, tx := range txs {
			_txStart     := time.Now()
			txNonce      := tx.Nonce()
			txHash       := tx.Hash()
			txHashStr    := txHash.String()
			canTransform := false

			if flag := _txhashMap[txHash]; flag {
				continue
			}

			if txNonce < nonce {
				log.Debug(">> nonce low", "from", addr, "tx_nonce", txNonce, "nonce", nonce, "hash", txHashStr)
				continue
			} else if txNonce == (nonce - 1) {
				log.Info(">> tx need replace", "from", addr, "nonce", txNonce, "hash", txHashStr)
				canTransform = true
			} else if txNonce == nonce {
				canTransform = true
			} else {
				log.Debug(">> nonce high, wait", "from", addr, "tx_nonce", txNonce, "nonce", nonce, "hash", txHashStr)
				break
			}

			if !canTransform {
				continue
			}

			if err := b.BroadcastTx(tx); err != nil {
				errStr := fmt.Sprintf("%s", err)
				if strings.Contains(errStr, "Tx already exists in cache") {
					log.Info(">> Tx already exists in cache, ignore it", "from", addr, "hash", txHashStr)
				} else {
					log.Error(">> BroadcastTx error", "err", err, "err_str", errStr, "from", addr, "tx_nonce", txNonce, "hash", txHashStr)
					break
				}
			}

			counts++
			nonce = tx.Nonce() + 1
			b.setNonce(addr, nonce)
			_txhashMap[txHash] = true
			log.Info(">> BroadcastTx", "from", addr, "tx_nonce", txNonce, "hash", txHashStr, "height", _blockNum.String(), "used", fmt.Sprintf("%dms", time.Since(_txStart)/time.Millisecond))
		}
	}

	if total > 1 {
		log.Info(">> transformTx end", "count", counts, "total", total, "height", _blockNum.String(), "used", fmt.Sprintf("%dms", time.Since(_start)/time.Millisecond))
	}
}

// BroadcastTx broadcasts a transaction to tendermint core
// #unstable
func (b *Backend) BroadcastTx(tx *ethTypes.Transaction) error {
	//var result interface{}
	result := ctypes.ResultBroadcastTx{}

	buf := new(bytes.Buffer)
	if err := tx.EncodeRLP(buf); err != nil {
		return err
	}

	// transform special tx
	etx := lkTypes.EtherTx{Type: "tx", Tx: hexutil.Encode(buf.Bytes())}
	jsonBytes, err := json.Marshal(etx)
	if err != nil {
		return err
	}

	//log.Info("BroadcastTx:", "EtherTx", string(jsonBytes))
	params := map[string]interface{}{
		"tx": hexutil.Encode(jsonBytes),
	}
	_, er := b.client.Call("broadcast_tx_sync", params, &result)
	if er != nil {
		return er
	}

	if result.Code != 0 {
		return fmt.Errorf("%s", result.Log)
	}

	return nil
}

//----------------------------------------------------------------------
// wait for Tendermint to open the socket and run http endpoint

func waitForServer(c rpcClient.HTTPClient) {
	var result interface{}
	for {
		_, err := c.Call("status", map[string]interface{}{}, &result)
		if err == nil {
			break
		}

		log.Info("Waiting for tendermint endpoint to start", "err", err)
		time.Sleep(time.Second * 3)
	}
}
