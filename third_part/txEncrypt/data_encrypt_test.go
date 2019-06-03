package data_encrypt

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenPubkey(t *testing.T) {
	assert := assert.New(t)

	// create some fixed values
	addrStr := "5A16CD497EC286A8DDABDE4E1D19795A42161F56"
	pubStr := "0139560D39C3654A0506068C6E6701BC931757C96E1272FD1632979044E7E039EE"
	addrBytes, _ := hex.DecodeString(addrStr)

	// prepend type byte
	pubKey, err := GenPubkey(pubStr)
	assert.NoError(err)

	// make sure the values match
	assert.EqualValues(addrBytes, pubKey.Address())
}

func TestGenPrikey(t *testing.T) {
	assert := assert.New(t)

	// create some fixed values
	pubStr := "0139560D39C3654A0506068C6E6701BC931757C96E1272FD1632979044E7E039EE"
	privStr := "0114160065BA2E54F297FD1AB25C4A484687F5FD98C664D0D14FD7B681351A7C7039560D39C3654A0506068C6E6701BC931757C96E1272FD1632979044E7E039EE"

	// prepend type byte
	pubKey, err := GenPubkey(pubStr)
	assert.NoError(err)
	privKey, err := GenPrikey(privStr)
	assert.NoError(err)

	// make sure the values match
	assert.EqualValues(*pubKey, privKey.PubKey())
}

func TestSign(t *testing.T) {
	assert := assert.New(t)

	// create some fixed values
	pubStr := "0139560D39C3654A0506068C6E6701BC931757C96E1272FD1632979044E7E039EE"
	privStr := "0114160065BA2E54F297FD1AB25C4A484687F5FD98C664D0D14FD7B681351A7C7039560D39C3654A0506068C6E6701BC931757C96E1272FD1632979044E7E039EE"

	data := []byte("1234")
	sign, err :=  Sign(privStr, data)
	assert.NoError(err)
	ret := VerifySign(pubStr, data, sign)
	assert.True(ret)
}