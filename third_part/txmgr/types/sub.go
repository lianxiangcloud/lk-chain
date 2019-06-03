package types

type TxRecordReq struct {
	TxHash string `json:"tx_hash"`
	Type   string `json:"type"`
}
