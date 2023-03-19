package mock

import (
	"context"

	ctypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sisu-network/lib/log"
	dTypes "github.com/sodiumlabs/dheart/types"
)

type DheartClient struct {
	client *rpc.Client
}

// DialDheart connects a client to the given URL.
func DialDheart(rawurl string) (*DheartClient, error) {
	return dialDheartContext(context.Background(), rawurl)
}

func dialDheartContext(ctx context.Context, rawurl string) (*DheartClient, error) {
	c, err := rpc.DialContext(ctx, rawurl)
	if err != nil {
		return nil, err
	}
	return newDheartClient(c), nil
}

func newDheartClient(c *rpc.Client) *DheartClient {
	return &DheartClient{c}
}

func (c *DheartClient) SetPrivKey(encodedKey string, keyType string) error {
	var result string
	err := c.client.CallContext(context.Background(), &result, "tss_setPrivKey", encodedKey, keyType)
	if err != nil {
		log.Error("Cannot do set private key with dheart, err = ", err)
		return err
	}

	return nil
}

func (c *DheartClient) Ping(source string) error {
	var result interface{}
	err := c.client.CallContext(context.Background(), &result, "tss_ping", source)
	if err != nil {
		log.Error("Cannot ping sisu, err = ", err)
		return err
	}

	return nil
}

func (c *DheartClient) SetSisuReady(isReady bool) error {
	var r interface{}
	err := c.client.CallContext(context.Background(), &r, "tss_setSisuReady", isReady)
	if err != nil {
		log.Error("Cannot SetSisuReady, err = ", err)
		return err
	}

	return nil
}

func (c *DheartClient) KeyGen(keygenId string, keygenType string, pubKeys []ctypes.PubKey) error {
	// Wrap pubkeys
	wrappers := make([]dTypes.PubKeyWrapper, len(pubKeys))
	for i, pubKey := range pubKeys {

		switch pubKey.Type() {
		case "ed25519":
			wrappers[i] = dTypes.PubKeyWrapper{
				KeyType: pubKey.Type(),
				Key:     pubKey.Bytes(),
			}
		case "secp256k1":
			wrappers[i] = dTypes.PubKeyWrapper{
				KeyType: pubKey.Type(),
				Key:     pubKey.Bytes(),
			}
		}
	}

	var result string
	err := c.client.CallContext(context.Background(), &result, "tss_keyGen", keygenId, keygenType, wrappers)
	if err != nil {
		log.Error("Cannot send keygen request, err = ", err)
		return err
	}

	return nil
}

func (c *DheartClient) KeySign(req *dTypes.KeysignRequest, pubKeys []ctypes.PubKey) error {
	log.Verbose("Broadcasting key signing to Dheart")

	// Wrap pubkeys
	wrappers := make([]dTypes.PubKeyWrapper, len(pubKeys))
	for i, pubKey := range pubKeys {

		switch pubKey.Type() {
		case "ed25519":
			wrappers[i] = dTypes.PubKeyWrapper{
				KeyType: pubKey.Type(),
				Key:     pubKey.Bytes(),
			}
		case "secp256k1":
			wrappers[i] = dTypes.PubKeyWrapper{
				KeyType: pubKey.Type(),
				Key:     pubKey.Bytes(),
			}
		}
	}

	var r interface{}
	err := c.client.CallContext(context.Background(), &r, "tss_keySign", req, wrappers)
	if err != nil {
		log.Error("Cannot send KeySign request, err = ", err)
		return err
	}

	return nil
}
