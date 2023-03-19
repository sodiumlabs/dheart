package cache

import (
	"sync"

	"github.com/sodiumlabs/dheart/tools"
	"github.com/sodiumlabs/dheart/types/common"
	"github.com/sodiumlabs/tss-lib/tss"
)

// A cache that saves all messages (with max size) per node during worker execution.
type WorkMessageCache struct {
	cache       map[string]tools.CircularQueue
	sizePerNode int
	lock        *sync.RWMutex
	myPid       *tss.PartyID
}

func NewWorkMessageCache(sizePerNode int, myPid *tss.PartyID) *WorkMessageCache {
	return &WorkMessageCache{
		cache:       make(map[string]tools.CircularQueue),
		sizePerNode: sizePerNode,
		myPid:       myPid,
		lock:        &sync.RWMutex{},
	}
}

func (c *WorkMessageCache) Add(key string, msg *common.SignedMessage) {
	c.lock.Lock()
	defer c.lock.Unlock()

	q := c.cache[msg.From]
	if q == nil {
		if msg.From == c.myPid.Id {
			q = tools.NewCircularQueue(c.sizePerNode * 5)
		} else {
			q = tools.NewCircularQueue(c.sizePerNode)
		}
	}

	q.Add(key, msg)
	c.cache[msg.From] = q
}

func (c *WorkMessageCache) Get(from, key string) *common.SignedMessage {
	c.lock.RLock()
	defer c.lock.RUnlock()

	q := c.cache[from]
	if q == nil {
		return nil
	}

	item := q.Get(key)
	if item == nil {
		return nil
	}

	return item.(*common.SignedMessage)
}
