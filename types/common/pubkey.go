package common

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	"github.com/tendermint/tendermint/crypto"
	tmsecp256k1 "github.com/tendermint/tendermint/crypto/secp256k1"
	"gopkg.in/yaml.v2"

	"github.com/maticnetwork/bor/common"
	"github.com/maticnetwork/bor/common/hexutil"
)

// PubKey pubkey
type PubKey [65]byte

// ZeroPubKey represents empty pub key
var ZeroPubKey = PubKey{}

// NewPubKey from byte array
func NewPubKey(data []byte) PubKey {
	var key PubKey
	copy(key[:], data[:])
	return key
}

// MarshalText returns the hex representation of a.
func (a PubKey) MarshalText() ([]byte, error) {
	return hexutil.Bytes(a[:]).MarshalText()
}

// UnmarshalText parses a hash in hex syntax.
func (a *PubKey) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("PubKey", input, a[:])
}

// MarshalTo
func (m *PubKey) MarshalTo(dAtA []byte) (int, error) {
	return 0, nil
}

// MarshalTo
func (m *PubKey) Size() int {
	return binary.Size(m)
}

// String returns string representatin of key
func (a PubKey) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

// Bytes returns bytes for pubkey
func (a PubKey) Bytes() []byte {
	return a[:]
}

// Address returns address
func (a PubKey) Address() common.Address {
	return common.BytesToAddress(a.CryptoPubKey().Address().Bytes())
}

// CryptoPubKey returns crypto pub key for tendermint
func (a PubKey) CryptoPubKey() crypto.PubKey {
	return tmsecp256k1.PubKey(a[:])
	// return &secp256k1.PubKey{Key: a.Bytes()}
}

// TODO: check if any interface is implementing
// ABCIPubKey returns abci pubkey for cosmos
// func (a PubKey) ABCIPubKey() abci.PubKey {
// 	return tmTypes.TM2PB.PubKey(a.CryptoPubKey())
// }

// Marshal returns the raw address bytes. It is needed for protobuf compatibility.
func (a PubKey) Marshal() ([]byte, error) {
	return a.Bytes(), nil
}

func (a PubKey) StringToPubKey(input string) PubKey {
	return NewPubKey([]byte(input))
}

// Unmarshal sets the address to the given data. It is needed for protobuf
// compatibility.
func (a *PubKey) Unmarshal(data []byte) error {
	copy(a[:], data[:])
	return nil
}

// MarshalJSON marshals to JSON using Bech32.
func (a PubKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// MarshalYAML marshals to YAML using Bech32.
func (a PubKey) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

// UnmarshalJSON unmarshals from JSON assuming Bech32 encoding.
func (a *PubKey) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	copy(a[:], common.FromHex(s))
	return nil
}

// UnmarshalYAML unmarshals from JSON assuming Bech32 encoding.
func (a *PubKey) UnmarshalYAML(data []byte) error {
	var s string
	err := yaml.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	copy(a[:], common.FromHex(s))
	return nil
}
