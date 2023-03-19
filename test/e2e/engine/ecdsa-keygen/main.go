package main

import (
	"flag"
	"math/big"
	"time"

	"github.com/sisu-network/lib/log"
	"github.com/sodiumlabs/dheart/core"
	"github.com/sodiumlabs/dheart/core/config"
	"github.com/sodiumlabs/dheart/db"
	"github.com/sodiumlabs/dheart/p2p"
	thelper "github.com/sodiumlabs/dheart/test/e2e/helper"
	htypes "github.com/sodiumlabs/dheart/types"
	"github.com/sodiumlabs/dheart/worker"
	"github.com/sodiumlabs/dheart/worker/types"
	"github.com/sodiumlabs/tss-lib/tss"
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
}

func (cb *EngineCallback) OnWorkFailed(request *types.WorkRequest, culprits []*tss.PartyID) {
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

func main() {
	var index, n int
	var isSlowNode bool

	flag.IntVar(&index, "index", 0, "listening port")
	flag.IntVar(&n, "n", 2, "number of nodes")
	flag.BoolVar(&isSlowNode, "is-slow", false, "Use it when testing message caching mechanism")
	flag.Parse()

	cfg, privateKey := p2p.GetMockSecp256k1Config(n, index)
	cm := p2p.NewConnectionManager(cfg)
	if isSlowNode {
		cm = thelper.NewSlowConnectionManager(cfg)
	}
	err := cm.Start(privateKey, "secp256k1")

	if err != nil {
		panic(err)
	}

	pids := make([]*tss.PartyID, n)
	allKeys := p2p.GetAllSecp256k1PrivateKeys(n)
	nodes := make([]*core.Node, n)

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
	outCh := make(chan *htypes.KeygenResult)
	cb := NewEngineCallback(outCh, nil, nil)
	engine := core.NewEngine(nodes[index], cm, db.NewMockDatabase(), cb, allKeys[index], config.NewDefaultTimeoutConfig())
	cm.AddListener(p2p.TSSProtocolID, engine)

	// Add nodes
	for i := 0; i < n; i++ {
		engine.AddNodes(nodes)
	}

	time.Sleep(time.Second * 3)

	// Add request
	workId := "keygen0"
	request := types.NewEcKeygenRequest("ecdsa", workId, pids, n-1, worker.LoadEcPreparams(index))
	err = engine.AddRequest(request)
	if err != nil {
		panic(err)
	}

	select {
	case result := <-outCh:
		log.Info("Result ", result)
	}
}
