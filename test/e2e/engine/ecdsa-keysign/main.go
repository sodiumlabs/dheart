package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"

	"math/rand"

	ipfslog "github.com/ipfs/go-log"

	"flag"
	"fmt"
	"math/big"
	"time"

	libchain "github.com/sisu-network/lib/chain"
	thelper "github.com/sodiumlabs/dheart/test/e2e/helper"
	"github.com/sodiumlabs/dheart/utils"
	"github.com/sodiumlabs/dheart/worker"

	"github.com/sisu-network/lib/log"
	"github.com/sodiumlabs/dheart/core"
	"github.com/sodiumlabs/dheart/core/config"
	"github.com/sodiumlabs/dheart/db"
	"github.com/sodiumlabs/dheart/p2p"
	htypes "github.com/sodiumlabs/dheart/types"
	"github.com/sodiumlabs/dheart/worker/types"
	"github.com/sodiumlabs/tss-lib/tss"

	ctypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

type EngineCallback struct {
	keygenDataCh  chan *htypes.KeygenResult
	presignDataCh chan *htypes.PresignResult
	signingDataCh chan *htypes.KeysignResult
}

func NewEngineCallback(
	keygenDataCh chan *htypes.KeygenResult,
	presignDataCh chan *htypes.PresignResult,
	signingDataCh chan *htypes.KeysignResult,
) *EngineCallback {
	return &EngineCallback{
		keygenDataCh, presignDataCh, signingDataCh,
	}
}

func (cb *EngineCallback) OnWorkKeygenFinished(result *htypes.KeygenResult) {
	cb.keygenDataCh <- result
}

func (cb *EngineCallback) OnWorkPresignFinished(result *htypes.PresignResult) {
	cb.presignDataCh <- result
}

func (cb *EngineCallback) OnWorkSigningFinished(request *types.WorkRequest, result *htypes.KeysignResult) {
	if cb.signingDataCh != nil {
		msgs := make([]*htypes.KeysignMessage, 0)
		for i, bz := range request.Messages {
			msgs = append(msgs, &htypes.KeysignMessage{
				Bytes:       bz,
				BytesToSign: bz,
				OutChain:    request.Chains[i],
			})
		}
		result.Request = &htypes.KeysignRequest{
			KeyType:         "ecdsa",
			KeysignMessages: msgs,
		}
		cb.signingDataCh <- result
	}
}

func (cb *EngineCallback) OnWorkFailed(request *types.WorkRequest, culprits []*tss.PartyID) {
	if cb.signingDataCh != nil {
		cb.signingDataCh <- nil
	}
}

func getSortedPartyIds(n int) tss.SortedPartyIDs {
	keys := p2p.GetAllSecp256k1PrivateKeys(n)
	partyIds := make([]*tss.PartyID, n)

	// Creates list of party ids
	for i := 0; i < n; i++ {
		bz := keys[i].PubKey().Bytes()
		peerId := p2p.P2PIDFromKey(keys[i])
		party := tss.NewPartyID(peerId.String(), "", new(big.Int).SetBytes(bz))
		partyIds[i] = party
	}

	return tss.SortPartyIDs(partyIds, 0)
}

func getDb(index int) db.Database {
	dbConfig := config.GetLocalhostDbConfig()
	dbConfig.Schema = fmt.Sprintf("dheart%d", index)
	dbConfig.InMemory = true

	dbInstance := db.NewDatabase(&dbConfig)

	err := dbInstance.Init()
	if err != nil {
		panic(err)
	}

	return dbInstance
}

func doKeygen(pids tss.SortedPartyIDs, index int, engine core.Engine,
	outCh chan *htypes.KeygenResult) *htypes.KeygenResult {
	// Add request
	workId := "keygen0"
	threshold := utils.GetThreshold(len(pids))
	request := types.NewEcKeygenRequest("ecdsa", workId, pids, threshold, worker.LoadEcPreparams(index))
	err := engine.AddRequest(request)
	if err != nil {
		panic(err)
	}

	var result *htypes.KeygenResult
	select {
	case result = <-outCh:
	case <-time.After(time.Second * 100):
		panic("Keygen timeout")
	}

	return result
}

func verifySignature(pubkey *ecdsa.PublicKey, msg []byte, R, S *big.Int) {
	ok := ecdsa.Verify(pubkey, msg, R, S)
	if !ok {
		panic(fmt.Sprintf("Signature verification fails for msg: %s", hex.EncodeToString(msg)))
	}
}

func verifyKeysignResult(index int, testCount int, keysignch chan *htypes.KeysignResult,
	keysignInput *htypes.KeygenResult) {

	for i := 0; i < testCount; i++ {
		var result *htypes.KeysignResult
		select {
		case result = <-keysignch:
		case <-time.After(time.Second * 100):
			panic("Signing timeout")
		}

		if result == nil {
			panic("result is nil")
		}

		message := result.Request.KeysignMessages[0].BytesToSign

		switch result.Outcome {
		case htypes.OutcomeSuccess:
			for i, msg := range result.Request.KeysignMessages {
				x, y := elliptic.Unmarshal(tss.EC("ecdsa"), keysignInput.PubKeyBytes)
				pk := ecdsa.PublicKey{
					Curve: tss.EC("ecdsa"),
					X:     x,
					Y:     y,
				}

				sig := result.Signatures[i]
				if len(sig) != 65 {
					log.Info("Signature hex = ", hex.EncodeToString(sig))
					panic(fmt.Sprintf("Signature length is not correct. actual length = %d", len(sig)))
				}
				sig = sig[:64]

				r := sig[:32]
				s := sig[32:]

				verifySignature(&pk, msg.BytesToSign, new(big.Int).SetBytes(r), new(big.Int).SetBytes(s))
			}

			log.Infof("Signing succeeded for %s, index = %d, i = %d", hex.EncodeToString(message),
				index, i)

		case htypes.OutcomeFailure:
			panic("Failed to create signature")
		case htypes.OutcometNotSelected:
			log.Infof("Node is not selected for message %s, index = %d, i = %d",
				hex.EncodeToString(message), index, i)
		}
	}
}

func main() {
	ipfslog.SetLogLevel("dheart", "debug")

	var index, n, seed int
	var isSlow bool
	flag.IntVar(&index, "index", 0, "listening port")
	flag.IntVar(&n, "n", 2, "number of nodes in the test")
	flag.BoolVar(&isSlow, "is-slow", false, "Use it when testing message caching mechanism")
	flag.IntVar(&seed, "seed", 0, "seed for the test")
	flag.Parse()

	cfg, privateKey := p2p.GetMockSecp256k1Config(n, index)
	cm := p2p.NewConnectionManager(cfg)
	if isSlow {
		cm = thelper.NewSlowConnectionManager(cfg)
	} else {
		cm = cm.(*p2p.DefaultConnectionManager)
	}
	err := cm.Start(privateKey, "secp256k1")
	if err != nil {
		panic(err)
	}

	pids := make([]*tss.PartyID, n)
	allKeys := p2p.GetAllSecp256k1PrivateKeys(n)
	nodes := make([]*core.Node, n)
	tendermintPubKeys := make([]ctypes.PubKey, n)

	// Add nodes
	privKeys := p2p.GetAllSecp256k1PrivateKeys(n)
	for i := 0; i < n; i++ {
		pubKey := privKeys[i].PubKey()
		node := core.NewNode(pubKey)
		nodes[i] = node
		pids[i] = node.PartyId
	}
	pids = tss.SortPartyIDs(pids)

	// Create new engine
	keygenCh := make(chan *htypes.KeygenResult)
	keysignch := make(chan *htypes.KeysignResult)
	cb := NewEngineCallback(keygenCh, nil, keysignch)
	database := getDb(index)

	engine := core.NewEngine(nodes[index], cm, database, cb, allKeys[index],
		config.NewDefaultTimeoutConfig())
	cm.AddListener(p2p.TSSProtocolID, engine)

	// Add nodes
	for i := 0; i < n; i++ {
		engine.AddNodes(nodes)
		tendermintPubKeys[i] = privKeys[i].PubKey()
	}

	for {
		if cm.IsReady() {
			time.Sleep(time.Second * 3)
			break
		}
	}

	time.Sleep(time.Second * time.Duration(3+n/4))

	// Keygen
	keygenResult := doKeygen(pids, index, engine, keygenCh)

	presignInput, err := database.LoadEcKeygen(libchain.KEY_TYPE_ECDSA)
	if err != nil {
		panic(err)
	}

	// Keysign
	log.Info("Doing keysign now!")
	rand.Seed(int64(seed + 110))
	testCount := 10
	for i := 0; i < testCount; i++ {
		msg := make([]byte, 20)
		rand.Read(msg) //nolint
		if err != nil {
			panic(err)
		}
		log.Info("Msg hex = ", hex.EncodeToString(msg))

		workId := fmt.Sprintf("%s__%s", "keysign", hex.EncodeToString(msg))
		chains := []string{"eth"}
		threshold := utils.GetThreshold(len(pids))

		request := types.NewEcSigningRequest(workId, pids, threshold, [][]byte{msg}, chains,
			presignInput)

		err := engine.AddRequest(request)
		if err != nil {
			panic(err)
		}
	}

	verifyKeysignResult(index, testCount, keysignch, keygenResult)

	time.Sleep(time.Second * 2)

	fmt.Printf("%d %s Done!!!!\n", index, nodes[index].PartyId.Id)
}
