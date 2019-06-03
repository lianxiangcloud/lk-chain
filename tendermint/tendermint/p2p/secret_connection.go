// Uses nacl's secret_box to encrypt a net.Conn.
// It is (meant to be) an implementation of the STS protocol.
// Note we do not (yet) assume that a remote peer's pubkey
// is known ahead of time, and thus we are technically
// still vulnerable to MITM. (TODO!)
// See docs/sts-final.pdf for more info
package p2p

import (
	"bytes"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/ripemd160"

	"github.com/golang/snappy"
	"github.com/tendermint/go-crypto"
	"github.com/tendermint/go-wire"
	cmn "github.com/tendermint/tmlibs/common"
)

// frameHeaderSize + dataMaxSize == total frame size
const dataLenSize = 4 // uint32 to describe the length, is <= dataMaxSize
const encodeTypeSize = 1
const frameVersionSize = 1
const frameHeaderSize = dataLenSize + encodeTypeSize + frameVersionSize
const dataMaxSize = 32 * 1024
const totalFrameSize = dataMaxSize + frameHeaderSize
const sealedFrameSize = totalFrameSize + secretbox.Overhead
const authSigMsgSize = (32 + 1) + (64 + 1) // fixed size (length prefixed) byte arrays

const frameCapacity = 65535
const encodedFrameSize = totalFrameSize + 1024

const encryptFrameType = 1
const compressedFrameType = 2

// frame encode type: encrypt or compress
var frameEncodeType = compressedFrameType
var frameVersion = 1

// Implements net.Conn
type SecretConnection struct {
	conn       io.ReadWriteCloser
	recvBuffer []byte
	recvNonce  *[24]byte
	sendNonce  *[24]byte
	remPubKey  crypto.PubKeyEd25519
	shrSecret  *[32]byte // shared secret
}

// Performs handshake and returns a new authenticated SecretConnection.
// Returns nil if error in handshake.
// Caller should call conn.Close()
// See docs/sts-final.pdf for more information.
func MakeSecretConnection(conn io.ReadWriteCloser, locPrivKey crypto.PrivKeyEd25519) (*SecretConnection, error) {

	locPubKey := locPrivKey.PubKey().Unwrap().(crypto.PubKeyEd25519)

	// Generate ephemeral keys for perfect forward secrecy.
	locEphPub, locEphPriv := genEphKeys()

	// Write local ephemeral pubkey and receive one too.
	// NOTE: every 32-byte string is accepted as a Curve25519 public key
	// (see DJB's Curve25519 paper: http://cr.yp.to/ecdh/curve25519-20060209.pdf)
	remEphPub, err := shareEphPubKey(conn, locEphPub)
	if err != nil {
		return nil, err
	}

	// Compute common shared secret.
	shrSecret := computeSharedSecret(remEphPub, locEphPriv)

	// Sort by lexical order.
	loEphPub, hiEphPub := sort32(locEphPub, remEphPub)

	// Check if the local ephemeral public key
	// was the least, lexicographically sorted.
	locIsLeast := bytes.Equal(locEphPub[:], loEphPub[:])

	// Generate nonces to use for secretbox.
	recvNonce, sendNonce := genNonces(loEphPub, hiEphPub, locIsLeast)

	// Generate common challenge to sign.
	challenge := genChallenge(loEphPub, hiEphPub)

	// Construct SecretConnection.
	sc := &SecretConnection{
		conn:       conn,
		recvBuffer: nil,
		recvNonce:  recvNonce,
		sendNonce:  sendNonce,
		shrSecret:  shrSecret,
	}

	// Sign the challenge bytes for authentication.
	locSignature := signChallenge(challenge, locPrivKey)

	// Share (in secret) each other's pubkey & challenge signature
	authSigMsg, err := shareAuthSignature(sc, locPubKey, locSignature)
	if err != nil {
		return nil, err
	}
	remPubKey, remSignature := authSigMsg.Key, authSigMsg.Sig
	if !remPubKey.VerifyBytes(challenge[:], remSignature) {
		return nil, errors.New("Challenge verification failed")
	}

	// We've authorized.
	sc.remPubKey = remPubKey.Unwrap().(crypto.PubKeyEd25519)
	return sc, nil
}

// Returns authenticated remote pubkey
func (sc *SecretConnection) RemotePubKey() crypto.PubKeyEd25519 {
	return sc.remPubKey
}

// Writes encrypted frames of `sealedFrameSize`
// CONTRACT: data smaller than dataMaxSize is read atomically.
func (sc *SecretConnection) Write(data []byte) (n int, err error) {
	for 0 < len(data) {
		var chunk []byte
		if dataMaxSize < len(data) {
			chunk = data[:dataMaxSize]
			data = data[dataMaxSize:]
		} else {
			chunk = data
			data = nil
		}
		chunkLength := len(chunk)

		if frameEncodeType == encryptFrameType {
			var frame []byte = make([]byte, totalFrameSize)

			// put header
			binary.BigEndian.PutUint32(frame, uint32(chunkLength))
			frame[dataLenSize] = byte(frameEncodeType)
			frame[dataLenSize+encodeTypeSize] = byte(frameVersion)

			// data
			copy(frame[frameHeaderSize:], chunk)

			// encrypt the frame
			var sealedFrame = make([]byte, sealedFrameSize)
			secretbox.Seal(sealedFrame[:0], frame, sc.sendNonce, sc.shrSecret)
			// fmt.Printf("secretbox.Seal(sealed:%X,sendNonce:%X,shrSecret:%X\n", sealedFrame, sc.sendNonce, sc.shrSecret)
			incr2Nonce(sc.sendNonce)
			// end encryption

			_, err := sc.conn.Write(sealedFrame)
			if err != nil {
				return n, err
			} else {
				n += len(chunk)
			}
		} else {
			// compress data
			encodedChunk := snappy.Encode(nil, []byte(chunk))
			encodedChunkLen := len(encodedChunk)

			if encodedChunkLen > frameCapacity {
				cmn.PanicCrisis(fmt.Sprintf("encodedChunkLen is greater than frameCapacity, encodedChunkLen=%d, frameCapacity=%d", encodedChunkLen, frameCapacity))
			}

			//fmt.Printf("Compress Encode, chunkLength=%d encodedChunkLen=%d\n", chunkLength, encodedChunkLen)
			var frame []byte = make([]byte, encodedChunkLen+frameHeaderSize)

			binary.BigEndian.PutUint32(frame, uint32(encodedChunkLen))
			frame[dataLenSize] = byte(frameEncodeType)
			frame[dataLenSize+encodeTypeSize] = byte(frameVersion)

			copy(frame[frameHeaderSize:], encodedChunk)

			_, err := sc.conn.Write(frame)
			if err != nil {
				return n, err
			} else {
				n += len(chunk)
			}
		}

	}
	return
}

// CONTRACT: data smaller than dataMaxSize is read atomically.
func (sc *SecretConnection) Read(data []byte) (n int, err error) {
	if 0 < len(sc.recvBuffer) {
		n_ := copy(data, sc.recvBuffer)
		sc.recvBuffer = sc.recvBuffer[n_:]
		return
	}

	var chunk []byte
	if frameEncodeType == encryptFrameType {
		var frame []byte = make([]byte, totalFrameSize)

		sealedFrame := make([]byte, sealedFrameSize)
		_, err = io.ReadFull(sc.conn, sealedFrame)
		if err != nil {
			return
		}

		// decrypt the frame
		// fmt.Printf("secretbox.Open(sealed:%X,recvNonce:%X,shrSecret:%X\n", sealedFrame, sc.recvNonce, sc.shrSecret)
		_, ok := secretbox.Open(frame[:0], sealedFrame, sc.recvNonce, sc.shrSecret)
		if !ok {
			return n, errors.New("Failed to decrypt SecretConnection")
		}
		incr2Nonce(sc.recvNonce)
		// end decryption

		var chunkLength = binary.BigEndian.Uint32(frame) // read the first two bytes
		if chunkLength > dataMaxSize {
			return 0, errors.New("chunkLength is greater than dataMaxSize")
		}
		chunk = frame[frameHeaderSize : frameHeaderSize+chunkLength]
	} else {
		// compress data
		var frameHdr []byte = make([]byte, frameHeaderSize)
		_, err = io.ReadFull(sc.conn, frameHdr)
		if err != nil {
			return
		}

		var frameLen = binary.BigEndian.Uint32(frameHdr)
		if frameLen > dataMaxSize {
			return 0, errors.New("chunkLength is greater than dataMaxSize")
		}
		var encodedChunk []byte = make([]byte, frameLen)

		_, err = io.ReadFull(sc.conn, encodedChunk)
		if err != nil {
			return
		}

		chunk, err = snappy.Decode(nil, encodedChunk)
		if err != nil {
			return
		}
		chunkLength := len(chunk)
		if chunkLength > dataMaxSize {
			return 0, errors.New(fmt.Sprintf("chunkLength=%d is greater than dataMaxSize", chunkLength))
		}

		//fmt.Printf("Compress Decode, chunkLength=%d encodedChunkLen=%d\n", chunkLength, frameLen)
	}

	n = copy(data, chunk)
	sc.recvBuffer = chunk[n:]
	return
}

// Implements net.Conn
func (sc *SecretConnection) Close() error                  { return sc.conn.Close() }
func (sc *SecretConnection) LocalAddr() net.Addr           { return sc.conn.(net.Conn).LocalAddr() }
func (sc *SecretConnection) RemoteAddr() net.Addr          { return sc.conn.(net.Conn).RemoteAddr() }
func (sc *SecretConnection) SetDeadline(t time.Time) error { return sc.conn.(net.Conn).SetDeadline(t) }
func (sc *SecretConnection) SetReadDeadline(t time.Time) error {
	return sc.conn.(net.Conn).SetReadDeadline(t)
}
func (sc *SecretConnection) SetWriteDeadline(t time.Time) error {
	return sc.conn.(net.Conn).SetWriteDeadline(t)
}

func genEphKeys() (ephPub, ephPriv *[32]byte) {
	var err error
	ephPub, ephPriv, err = box.GenerateKey(crand.Reader)
	if err != nil {
		cmn.PanicCrisis("Could not generate ephemeral keypairs")
	}
	return
}

func shareEphPubKey(conn io.ReadWriteCloser, locEphPub *[32]byte) (remEphPub *[32]byte, err error) {
	var err1, err2 error

	cmn.Parallel(
		func() {
			_, err1 = conn.Write(locEphPub[:])
		},
		func() {
			remEphPub = new([32]byte)
			_, err2 = io.ReadFull(conn, remEphPub[:])
		},
	)

	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}

	return remEphPub, nil
}

func computeSharedSecret(remPubKey, locPrivKey *[32]byte) (shrSecret *[32]byte) {
	shrSecret = new([32]byte)
	box.Precompute(shrSecret, remPubKey, locPrivKey)
	return
}

func sort32(foo, bar *[32]byte) (lo, hi *[32]byte) {
	if bytes.Compare(foo[:], bar[:]) < 0 {
		lo = foo
		hi = bar
	} else {
		lo = bar
		hi = foo
	}
	return
}

func genNonces(loPubKey, hiPubKey *[32]byte, locIsLo bool) (recvNonce, sendNonce *[24]byte) {
	nonce1 := hash24(append(loPubKey[:], hiPubKey[:]...))
	nonce2 := new([24]byte)
	copy(nonce2[:], nonce1[:])
	nonce2[len(nonce2)-1] ^= 0x01
	if locIsLo {
		recvNonce = nonce1
		sendNonce = nonce2
	} else {
		recvNonce = nonce2
		sendNonce = nonce1
	}
	return
}

func genChallenge(loPubKey, hiPubKey *[32]byte) (challenge *[32]byte) {
	return hash32(append(loPubKey[:], hiPubKey[:]...))
}

func signChallenge(challenge *[32]byte, locPrivKey crypto.PrivKeyEd25519) (signature crypto.SignatureEd25519) {
	signature = locPrivKey.Sign(challenge[:]).Unwrap().(crypto.SignatureEd25519)
	return
}

type authSigMessage struct {
	Key crypto.PubKey
	Sig crypto.Signature
}

func shareAuthSignature(sc *SecretConnection, pubKey crypto.PubKeyEd25519, signature crypto.SignatureEd25519) (*authSigMessage, error) {
	var recvMsg authSigMessage
	var err1, err2 error

	cmn.Parallel(
		func() {
			msgBytes := wire.BinaryBytes(authSigMessage{pubKey.Wrap(), signature.Wrap()})
			_, err1 = sc.Write(msgBytes)
		},
		func() {
			readBuffer := make([]byte, authSigMsgSize)
			_, err2 = io.ReadFull(sc, readBuffer)
			if err2 != nil {
				return
			}
			n := int(0) // not used.
			recvMsg = wire.ReadBinary(authSigMessage{}, bytes.NewBuffer(readBuffer), authSigMsgSize, &n, &err2).(authSigMessage)
		})

	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}

	return &recvMsg, nil
}

//--------------------------------------------------------------------------------

// sha256
func hash32(input []byte) (res *[32]byte) {
	hasher := sha256.New()
	hasher.Write(input) // nolint: errcheck, gas
	resSlice := hasher.Sum(nil)
	res = new([32]byte)
	copy(res[:], resSlice)
	return
}

// We only fill in the first 20 bytes with ripemd160
func hash24(input []byte) (res *[24]byte) {
	hasher := ripemd160.New()
	hasher.Write(input) // nolint: errcheck, gas
	resSlice := hasher.Sum(nil)
	res = new([24]byte)
	copy(res[:], resSlice)
	return
}

// increment nonce big-endian by 2 with wraparound.
func incr2Nonce(nonce *[24]byte) {
	incrNonce(nonce)
	incrNonce(nonce)
}

// increment nonce big-endian by 1 with wraparound.
func incrNonce(nonce *[24]byte) {
	for i := 23; 0 <= i; i-- {
		nonce[i] += 1
		if nonce[i] != 0 {
			return
		}
	}
}
