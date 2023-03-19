package blame

import (
	"fmt"
	"sync"

	mapset "github.com/deckarep/golang-set"
	"github.com/sodiumlabs/tss-lib/tss"
)

// Manager keeps track of the nodes that have sent messages in each round to find the culprits if not successful.
// It also keeps track of pre_execution culprits in keysign and presign round.
type Manager struct {
	// For each round, check which nodes sent message

	// Key: workID + round, value: set of sent nodes
	sentNodes map[string]map[string]struct{}

	// Key: workID + round, value: set of culprits in that round.
	roundCulprits        map[string][]*tss.PartyID
	preExecutionCulprits []*tss.PartyID

	mgrLock *sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		sentNodes:     make(map[string]map[string]struct{}),
		roundCulprits: make(map[string][]*tss.PartyID),
		mgrLock:       &sync.RWMutex{},
	}
}

func (m *Manager) AddSender(workId string, round uint32, sender string) {
	m.mgrLock.Lock()
	defer m.mgrLock.Unlock()

	key := createRoundKey(workId, round)
	if _, ok := m.sentNodes[key]; !ok {
		m.sentNodes[key] = make(map[string]struct{})
	}

	m.sentNodes[key][sender] = struct{}{}
}

func (m *Manager) AddPreExecutionCulprit(culprits []*tss.PartyID) {
	m.mgrLock.Lock()
	defer m.mgrLock.Unlock()

	m.preExecutionCulprits = append(m.preExecutionCulprits, culprits...)
}

func (m *Manager) AddCulpritByRound(workID string, round uint32, culprits []*tss.PartyID) {
	m.mgrLock.Lock()
	defer m.mgrLock.Unlock()

	key := createRoundKey(workID, round)
	m.roundCulprits[key] = append(m.roundCulprits[key], culprits...)
}

func (m *Manager) GetRoundCulprits(workId string, round uint32, allParties map[string]*tss.PartyID) []*tss.PartyID {
	m.mgrLock.RLock()
	defer m.mgrLock.RUnlock()

	key := createRoundKey(workId, round)
	sentNodes := mapset.NewSet()
	for node := range m.sentNodes[key] {
		sentNodes.Add(node)
	}

	allPeers := mapset.NewSet()
	for _, p := range allParties {
		allPeers.Add(p.Id)
	}

	// Nodes that have not sent messages.
	diff := allPeers.Difference(sentNodes)

	// Merge with nodes that have sent error messages, and deduplicate.
	for _, pid := range m.roundCulprits[key] {
		diff.Add(pid.Id)
	}

	culprits := make([]*tss.PartyID, 0, diff.Cardinality())
	for _, p := range diff.ToSlice() {
		if v, ok := allParties[p.(string)]; ok {
			culprits = append(culprits, v)
		}
	}

	return culprits
}

func (m *Manager) GetPreExecutionCulprits() []*tss.PartyID {
	m.mgrLock.RLock()
	defer m.mgrLock.RUnlock()

	return m.preExecutionCulprits
}

func createRoundKey(workID string, round uint32) string {
	return fmt.Sprintf("%s:%d", workID, round)
}
