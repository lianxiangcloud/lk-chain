package types

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync/atomic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/sha3"

	"third_part/lkcrypto"
)

type WrappedLKTransaction struct {
	LKx      *LKTransaction `json:"lkx"`
	BlockNum *big.Int       `json:"block_num"`
}

func (w *WrappedLKTransaction) String() string {
	return fmt.Sprintf(`lkx=%s, blockNum=%v`, w.LKx.String(), w.BlockNum)
}

type EtherTx struct {
	Type string `json:"type"`
	Tx   string `json:"tx"`
}

func FromEtherTx(txHexBytes string) (*LKTransaction, error) {
	jsonBytes, err := hexutil.Decode(txHexBytes)
	if err != nil {
		return nil, fmt.Errorf("hexutil.Decode: %v", err)
	}
	var etherTx EtherTx
	var lkx = &LKTransaction{}

	if err := json.Unmarshal(jsonBytes, &etherTx); err != nil {
		return nil, err
	}
	switch etherTx.Type {
	case LKTxType:
		if err := lkx.Decode(etherTx.Tx); err != nil {
			return nil, fmt.Errorf("Decode: %v", err)
		}
	case LKRawTxType:
		tx, err := decodeRawTxString(etherTx.Tx)
		if err != nil {
			return nil, fmt.Errorf("decodeRawTxString: %v", err)
		}
		var etx = &EthTransaction{
			Tx: tx,
		}
		lkx.ETx = etx
		lkx.Type = LKRawTxType
	case LKValidatorType:
	default:
		return nil, ErrUnknownEtherTxType
	}

	return lkx, nil
}

type EthTransaction struct {
	Tx      *types.Transaction `json:"tx"`
	GasUsed *big.Int           `json:"gas_used"`
	// Result: 0(success), others(fail, details come with Errmsg field)
	Ret    int    `json:"ret"`
	Errmsg string `json:"errmsg"`
}

func (tx *EthTransaction) TxHash() common.Hash {
	return tx.Hash()
}

func (tx *EthTransaction) Hash() common.Hash {
	return tx.Tx.Hash()
}

func (tx *EthTransaction) String() string {
	return fmt.Sprintf("{gas_used=%s, ret=%d, errmsg=%s, %s}", tx.GasUsed, tx.Ret, tx.Errmsg, tx.Tx)
}

type ContractTransaction struct {
	TxHash   common.Hash    `json:"tx_hash"`
	From     common.Address `json:"from"`
	To       common.Address `json:"address"`
	Value    *big.Int       `json:"value"`
	Index    int            `json:"index"`
	BlockNum *big.Int       `json:"block_num"`
	hash     atomic.Value
}

func (tx *ContractTransaction) String() string {
	return fmt.Sprintf("{tx_hash=%s, from=%s, to=%s. value=%s, index=%d}", tx.TxHash.String(),
		tx.From.String(), tx.To.String(), tx.Value.String(), tx.Index)
}

func (tx *ContractTransaction) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := jsonHash([]interface{}{tx.TxHash, tx.From, tx.To, tx.Value, tx.Index})
	if tx.Index != 0 {
		tx.hash.Store(v)
	}
	return v
}

type SuicideTransaction struct {
	TxHash   common.Hash    `json:"tx_hash"`
	From     common.Address `json:"from"`
	To       common.Address `json:"to"`
	Target   common.Address `json:"target"`
	Index    int            `json:"index"`
	Value    *big.Int       `json:"value"`
	BlockNum *big.Int       `json:"block_num"`
	hash     atomic.Value
}

func (tx *SuicideTransaction) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := jsonHash([]interface{}{tx.TxHash, tx.From, tx.To, tx.Value, tx.Target, tx.Index})
	if tx.Index != 0 {
		tx.hash.Store(v)
	}
	return v
}

func (tx *SuicideTransaction) String() string {
	return fmt.Sprintf("{tx_hash=%s, from=%s, to=%s, target=%s, value=%s, index=%d}", tx.TxHash.String(),
		tx.From.String(), tx.To.String(), tx.Target.String(), tx.Value.String(), tx.Index)
}

type LKTransaction struct {
	Type string               `json:"type"`
	ETx  *EthTransaction      `json:"etx"`
	CTx  *ContractTransaction `json:"ctx"`
	STx  *SuicideTransaction  `json:"stx"`
	Sign string               `json:"sign"`
}

func (d *LKTransaction) BlockNum() *big.Int {
	switch d.Type {
	case ContractTxType:
		return d.CTx.BlockNum
	case SuicideTxType:
		return d.STx.BlockNum
	default:
		return nil
	}
}

func (d *LKTransaction) String() string {
	switch d.Type {
	case EthTxType:
		return fmt.Sprintf("{type=%s, tx=%s, sign=%s}", d.Type, d.ETx.String(), d.Sign)
	case ContractTxType:
		return fmt.Sprintf("{type=%s, tx=%s, sign=%s}", d.Type, d.CTx.String(), d.Sign)
	case SuicideTxType:
		return fmt.Sprintf("{type=%s, tx=%s, sign=%s}", d.Type, d.STx.String(), d.Sign)
	default:
		return ""
	}
}

// "external" transaction encoding. used for signature, etc.
type extTransaction struct {
	Type string               `json:"type"`
	Etx  *EthTransaction      `json:"etx,omitempty"`
	Ctx  *ContractTransaction `json:"ctx,omitempty"`
	Stx  *SuicideTransaction  `json:"stx,omitempty"`
}

func (tx *LKTransaction) message() ([]byte, error) {
	return json.Marshal(extTransaction{
		Type: tx.Type,
		Etx:  tx.ETx,
		Ctx:  tx.CTx,
		Stx:  tx.STx,
	})
}

// SignTx signs the transaction using the given private key
func SignTx(tx *LKTransaction, prv string) (*LKTransaction, error) {
	if tx == nil {
		return nil, ErrInvalidTx
	}
	msg, err := tx.message()
	if err != nil {
		return nil, err
	}
	sig, err := lkcrypto.SignWithHexPrivkey(prv, msg)
	if err != nil {
		return nil, fmt.Errorf("SignWithHexPrivkey: %v", err)
	}
	return tx.withSignature(sig)
}

func CheckTxSign(tx *LKTransaction, pub string) error {
	if tx == nil {
		return ErrInvalidTx
	}
	msg, err := tx.message()
	if err != nil {
		return fmt.Errorf("Message: %v", err)
	}

	sig, err := hexutil.Decode(tx.Sign)
	if err != nil {
		return fmt.Errorf("hexutil.Decode: %v", err)
	}

	ok, err := lkcrypto.VerifySignWithHexPubkey(pub, msg, sig)
	if err != nil {
		return fmt.Errorf("VerifySignWithHexPubkey: %v", err)
	}
	if !ok {
		return ErrInvalidSignature
	}
	return nil
}

// withSignature returns a new transaction with the given signature.
func (tx *LKTransaction) withSignature(sig []byte) (*LKTransaction, error) {
	s := hexutil.Encode(sig)
	cpy := &LKTransaction{
		Type: tx.Type,
		ETx:  tx.ETx,
		CTx:  tx.CTx,
		STx:  tx.STx,
	}
	cpy.Sign = s
	return cpy, nil
}

func (tx *LKTransaction) TxHash() common.Hash {
	switch tx.Type {
	case EthTxType:
		if tx.ETx == nil {
			break
		}
		return tx.ETx.TxHash()
	case ContractTxType:
		if tx.CTx == nil {
			break
		}
		return tx.CTx.TxHash
	case SuicideTxType:
		if tx.STx == nil {
			break
		}
		return tx.STx.TxHash
	default:
		return common.Hash{}
	}
	return common.Hash{}
}

func (tx *LKTransaction) Hash() common.Hash {
	switch tx.Type {
	case EthTxType:
		return tx.ETx.Hash()
	case ContractTxType:
		return tx.CTx.Hash()
	case SuicideTxType:
		return tx.STx.Hash()
	default:
		return common.Hash{}
	}
}

func (tx *LKTransaction) Encode() string {
	data, _ := json.Marshal(tx)
	txWrap := EtherTx{
		Type: LKTxType,
		Tx:   string(data),
	}

	s, _ := json.Marshal(&txWrap)
	return hexutil.Encode(s)
}

func (tx *LKTransaction) Decode(s string) error {
	return json.Unmarshal([]byte(s), tx)
}

type LKBlock struct {
	Number       *big.Int         `json:"num"`
	Hash         common.Hash      `json:"hash"`
	Time         *big.Int         `json:"time"`
	Transactions []*LKTransaction `json:"txs"`
}

func (d *LKBlock) String() string {
	return fmt.Sprintf("{num=%s, hash=%s, txs=%d, time=%s}",
		d.Number.String(), d.Hash.String(), len(d.Transactions), d.Time.String())
}

func jsonHash(x interface{}) (h common.Hash) {
	b, _ := json.Marshal(x)
	hw := sha3.NewKeccak256()
	hw.Write(b[0:])
	hw.Sum(h[:0])
	return h
}
