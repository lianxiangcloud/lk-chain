package data_encrypt

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tendermint/go-crypto"
)

type ValidatorItemTx struct {
	Cmd    string `json:"cmd"`
	PubKey string `json:"pubkey"`
	Power  int64  `json:"power"`
	Name   string `json:"name"`
}

type ValidatorTx struct {
	Txs  []ValidatorItemTx `json:"item"`
	Sign map[string]string `json:"sign"`
}

func ToHex(b []byte) string {
	hex := Bytes2Hex(b)
	if len(hex) == 0 {
		hex = "0"
	}
	return "0x" + hex
}

func FromHex(s string) []byte {
	if len(s) > 1 {
		if s[0:2] == "0x" || s[0:2] == "0X" {
			s = s[2:]
		}
		if len(s)%2 == 1 {
			s = "0" + s
		}
		return Hex2Bytes(s)
	}
	return nil
}

func Bytes2Hex(d []byte) string {
	return hex.EncodeToString(d)
}

func Hex2Bytes(str string) []byte {
	h, _ := hex.DecodeString(str)

	return h
}

//keys:{prikey, pubkey}
func SignValidatorTx(txs ValidatorTx, keys map[string]string) (string, error) {
	if len(keys) == 0 {
		return "", errors.New("keys is empty")
	}

	if len(txs.Txs) == 0 {
		return "", errors.New("tx is empty")
	}

	txdata, err := json.Marshal(txs.Txs)
	if err != nil {
		return "", errors.New("txs.Txs Marshal error")
	}

	txs.Sign = make(map[string]string)
	for prikey, pubkey := range keys {
		sign, e := Sign(prikey, txdata)
		if e != nil {
			return "", errors.New("Sign error")
		}
		txs.Sign[pubkey] = ToHex(sign)
	}

	retdata, er := json.Marshal(txs)
	if er != nil {
		return "", errors.New("txs Marshal error")
	}

	return ToHex(retdata), nil
}

//pubkey:{pubkey, true}
func ParseValidatorTx(data string, pubkey map[string]int) (*ValidatorTx, error) {
	pubnum := len(pubkey)
	if pubnum == 0 {
		return nil, errors.New("pubkey list is empty")
	}

	databyte := FromHex(data)
	if databyte == nil {
		return nil, errors.New("data error")
	}

	var txs ValidatorTx
	err := json.Unmarshal(databyte, &txs)
	if err != nil {
		return nil, err
	}

	for _, v := range txs.Txs {
		// NOTE: expects go-wire encoded pubkey
		key, err := hex.DecodeString(v.PubKey)
		if err != nil {
			return nil, fmt.Errorf("Error decoding validator pubkey: err=%v, pubkey=%s", err, v.PubKey)
		}
		if _, err := crypto.PubKeyFromBytes(key); err != nil {
			return nil, fmt.Errorf("Error checking validator pubkey: err=%v, pubkey=%s", err, v.PubKey)
		}
	}

	txdata, e := json.Marshal(txs.Txs)
	if e != nil {
		return nil, e
	}

	var totalPower, signPower int64
	checkOk := false
	newPubKey := make(map[string]int)
	for k, v := range pubkey {
		newPubKey[strings.ToLower(k)] = v
		totalPower += int64(v)
	}

	threshold := totalPower * 1 / 3
	acc := int64(0)
	for _, v := range txs.Txs {
		power := int64(v.Power)
		// mind the overflow from int64
		if power < 0 {
			return nil, fmt.Errorf("Power (%d) overflows int64", v.Power)
		}
		cPower, ok := newPubKey[strings.ToLower(v.PubKey)]
		if !ok {
			acc = acc + power
		} else {
			np := int64(cPower) - power
			if np < 0 {
				np = -np
			}
			acc += np
		}
	}

	if acc >= threshold {
		return nil, fmt.Errorf("the change in voting power must be strictly less than 1/3, changePower=%d, threshold=%d, totalPower=%d", acc, threshold, totalPower)
	}

	for pkey, sign := range txs.Sign {
		ipower, ok := newPubKey[strings.ToLower(pkey)]
		if !ok {
			continue
		}

		if !VerifySign(pkey, txdata, FromHex(sign)) {
			continue
		}

		signPower += int64(ipower)
		if signPower > (totalPower * 2 / 3) {
			checkOk = true
			break
		}
	}

	if checkOk {
		return &txs, nil
	}

	return nil, errors.New("check sign error")
}
