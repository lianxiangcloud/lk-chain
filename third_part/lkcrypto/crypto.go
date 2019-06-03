package lkcrypto

import (
	"io/ioutil"
	"os"
	"encoding/hex"
	"errors"
	"fmt"

	gocrypto "third_part/lkcrypto/go-crypto"
)

func SignWithHexPrivkey(priv string, msg []byte) ([]byte, error) {
	key, err := HexToPrivkey(priv)
	if err != nil {
		return []byte{}, fmt.Errorf("HexToPrivkey: %v",err)
	}

	return key.Sign(msg).Bytes(), nil
}

func VerifySignWithHexPubkey(pub string, msg, sign []byte)(bool, error){
	key, err := HexToPubkey(pub)
	if err != nil {
		return false, fmt.Errorf("HexToPubkey: %v", err)
	}
	return VerifySign(*key, msg, sign), nil
}

func Sign(priv gocrypto.PrivKey, msg []byte) []byte {
	return priv.Sign(msg).Bytes()
}

func VerifySign(pubKey gocrypto.PubKey, msg, sign []byte) bool {
	sig, err := gocrypto.SignatureFromBytes(sign)
	if err != nil {
		return false
	}

	return pubKey.VerifyBytes(msg, sig)
}

func GenPrivKeySecp256k1(privateFile, publicFile string) error{
	privateKey := gocrypto.GenPrivKeySecp256k1()
	k := hex.EncodeToString(privateKey.Bytes())
	if err := ioutil.WriteFile(privateFile, []byte(k), 0600); err != nil {
		return err
	}

	k = hex.EncodeToString(privateKey.PubKey().Bytes())

	if err := ioutil.WriteFile(publicFile, []byte(k), 0600); err != nil {
		return err
	}
	return nil
}

func LoadPubkey(file string)(*gocrypto.PubKey, error){
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	buf, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	key, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}
	pubKey, err := gocrypto.PubKeyFromBytes(key)
	if err != nil {
		return nil, err
	}
	return &pubKey, nil
}

func LoadPrivkey(file string)(*gocrypto.PrivKey, error){
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	buf, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	key, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}
	privKey, err := gocrypto.PrivKeyFromBytes(key)
	return &privKey, err
}

func HexToPubkey(pub string)(*gocrypto.PubKey, error){
	key, err := hex.DecodeString(pub)
	if err != nil {
		return nil, errors.New("invalid hex string")
	}
	pubKey, err := gocrypto.PubKeyFromBytes(key)
	return &pubKey, err
}

func HexToPrivkey(priv string)(*gocrypto.PrivKey, error){
	key, err := hex.DecodeString(priv)
	if err != nil {
		return nil, errors.New("invalid hex string")
	}
	privKey, err := gocrypto.PrivKeyFromBytes(key)
	return &privKey, err
}