package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	ctypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/sisu-network/lib/log"

	libchain "github.com/sisu-network/lib/chain"

	"github.com/sodiumlabs/dheart/core"
	"github.com/sodiumlabs/dheart/core/config"
	"github.com/sodiumlabs/dheart/p2p"
	"github.com/sodiumlabs/dheart/run"
	"github.com/sodiumlabs/dheart/test/e2e/helper"
	"github.com/sodiumlabs/dheart/test/mock"
	"github.com/sodiumlabs/dheart/types"
	"github.com/sodiumlabs/dheart/utils"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
)

func getPublicKeys(n int) []ctypes.PubKey {
	pubKeys := make([]ctypes.PubKey, n)

	for i := 0; i < n; i++ {
		privKey := &secp256k1.PrivKey{Key: p2p.GetPrivateKeyBytes(i, "secp256k1")}
		pubKeys[i] = privKey.PubKey()
	}

	return pubKeys
}

func getEncrypted(privKey []byte) []byte {
	aesKey, err := hex.DecodeString(os.Getenv("AES_KEY_HEX"))
	if err != nil {
		panic(err)
	}

	// Encrypt with AES key
	encrypted, err := utils.AESDEncrypt(privKey, aesKey)
	if err != nil {
		panic(err)
	}

	return encrypted
}

func main() {
	var index, n int
	flag.IntVar(&index, "index", 0, "node index")
	flag.IntVar(&n, "n", 0, "Total nodes")
	flag.Parse()

	if n == 0 {
		n = 2
	}

	helper.ResetDb(index)

	run.LoadConfigEnv("../../../../.env")

	done := make(chan bool)
	presignResult := make(chan *types.PresignResult)
	signingResult := make(chan *types.KeysignResult)

	mockClient := &mock.MockClient{
		PostKeygenResultFunc: func(result *types.KeygenResult) error {
			done <- true
			return nil
		},
		PostPresignResultFunc: func(result *types.PresignResult) error {
			presignResult <- result
			return nil
		},

		PostKeysignResultFunc: func(result *types.KeysignResult) error {
			signingResult <- result
			return nil
		},
	}

	dbConfig := config.GetLocalhostDbConfig()
	dbConfig.Schema = fmt.Sprintf("dheart%d", index)

	cfg := config.HeartConfig{
		UseOnMemory: false,
		Db:          dbConfig,
	}

	conConfig, privKey := p2p.GetMockSecp256k1Config(n, index)
	cfg.Connection = conConfig

	aesKey, err := hex.DecodeString(os.Getenv("AES_KEY_HEX"))
	if err != nil {
		panic(err)
	}

	heartConfig := config.HeartConfig{
		Db:         dbConfig,
		AesKey:     aesKey,
		Connection: cfg.Connection,
	}
	pubkeys := getPublicKeys(n)

	heart := core.NewHeart(heartConfig, mockClient)
	if err := heart.Start(); err != nil {
		panic(err)
	}

	heart.SetBootstrappedKeys(pubkeys)

	encryptedKey := getEncrypted(privKey)
	err = heart.SetPrivKey(hex.EncodeToString(encryptedKey), "secp256k1")
	if err != nil {
		panic(err)
	}

	heart.SetSisuReady(true)
	heart.Keygen("keygenId", "ecdsa", pubkeys)
	select {
	case <-time.After(time.Second * 30):
		panic("Time out")
	case <-done:
	}

	// Presign
	heart.BlockEnd(0)
	select {
	case <-time.After(time.Second * 30):
		panic("Time out")
	case result := <-presignResult:
		if result.Outcome == types.OutcomeSuccess {
			log.Info("Presign Test Passed")
		} else {
			log.Info("Test result failed, culprits = ", result.Culprits)
			panic("Presigning failed")
		}
	}

	// Signing. Make sure that we don't create any new presign.
	keysignRequest := &types.KeysignRequest{
		KeyType: libchain.KEY_TYPE_ECDSA,
		KeysignMessages: []*types.KeysignMessage{
			{
				Id:          "keysign",
				OutChain:    "ganache1",
				BytesToSign: []byte("test"),
			},
		},
	}

	heart.Keysign(keysignRequest, pubkeys)

	select {
	case <-time.After(time.Second * 30):
		panic("Time out")
	case result := <-signingResult:
		if result.Outcome == types.OutcomeSuccess {
			log.Info("Singing Test Passed")
		} else {
			log.Info("Test signing result failed, culprits = ", result.Culprits)
			panic("Signing failed")
		}
	}
}
