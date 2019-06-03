package data_encrypt

import (
	"encoding/hex"

	crypto "github.com/tendermint/go-crypto"
)

func Sign(priv string, msg []byte) ([]byte, error) {
	prikey, err := GenPrikey(priv)
	if err != nil {
		return []byte(""), err
	}
	return prikey.Sign(msg).Bytes(), nil
}

func VerifySign(pub string, msg, sign []byte) bool {
	sig, err := GenSignature(sign)
	if err != nil {
		return false
	}

	pubkey, err := GenPubkey(pub)
	if err != nil {
		return false
	}

	return pubkey.VerifyBytes(msg, *sig)
}

func GenPubkey(pub string)(*crypto.PubKey, error) {
	key, err := hex.DecodeString(pub)
	if err != nil {
		return nil, err
	}

	pubKey, err := crypto.PubKeyFromBytes(key)
	if err != nil {
		return nil, err
	}
	return &pubKey, nil
}

func GenPrikey(priv string)(*crypto.PrivKey, error) {
	key, err := hex.DecodeString(priv)
	if err != nil {
		return nil, err
	}

	privKey, err := crypto.PrivKeyFromBytes(key)
	if err != nil {
		return nil, err
	}
	return &privKey, nil
}

func GenSignature(sign []byte)(*crypto.Signature, error) {
	csign, err := crypto.SignatureFromBytes(sign)
	if err != nil {
		return nil, err
	}
	return &csign, nil
}

