package components

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sisu-network/lib/log"
	"github.com/sodiumlabs/dheart/db"
	"github.com/sodiumlabs/dheart/types/common"
	"github.com/sodiumlabs/dheart/utils"
	ecsigning "github.com/sodiumlabs/tss-lib/ecdsa/signing"
	"github.com/sodiumlabs/tss-lib/tss"
)

type AvailablePresigns interface {
	Load() error
	GetAvailablePresigns(batchSize int, n int, allPids map[string]*tss.PartyID) ([]string, []*tss.PartyID)
	AddPresign(workId string, partyIds []*tss.PartyID, presignOutputs []*ecsigning.SignatureData_OneRoundData)
}

type defaultAvailablePresigns struct {
	db db.Database
	// Group all available presign by its list of pids.
	// map between: list of pids (string) -> array of available presigns.
	available map[string][]*common.AvailablePresign

	// Set of presign data that being used by a worker. In case the worker fails, we know which
	// nodes are using the presigns.
	// map between: list of pids (string) -> array of available presigns.
	lock *sync.RWMutex
}

func NewAvailPresignManager(db db.Database) AvailablePresigns {
	return &defaultAvailablePresigns{
		db:        db,
		available: make(map[string][]*common.AvailablePresign),
		lock:      &sync.RWMutex{},
	}
}

func (m *defaultAvailablePresigns) Load() error {
	presignIds, pidStrings, err := m.db.GetAvailablePresignShortForm()
	if err != nil {
		return err
	}

	m.lock.Lock()
	for i, pidString := range pidStrings {
		arr := m.available[pidString]
		if arr == nil {
			arr = make([]*common.AvailablePresign, 0)
		}

		ap := &common.AvailablePresign{
			PresignId:  presignIds[i],
			PidsString: pidString,
			Pids:       strings.Split(pidString, ","),
		}
		arr = append(arr, ap)
		m.available[pidString] = arr
	}
	m.lock.Unlock()

	return nil
}

func (m *defaultAvailablePresigns) AddPresign(workId string, partyIds []*tss.PartyID, presignOutputs []*ecsigning.SignatureData_OneRoundData) {
	if err := m.db.SavePresignData(workId, partyIds, presignOutputs); err != nil {
		log.Error("error when saving presign data", err)

		return
	}

	pids := utils.GetPidsArray(partyIds)
	pidString := utils.GetPidString(partyIds)

	// Add this to on-memory. TODO: Control the number of on-memory presign items size.
	arr := make([]*common.AvailablePresign, len(presignOutputs))

	for i := range presignOutputs {
		presignId := fmt.Sprintf("%s-%d", workId, i)

		arr[i] = &common.AvailablePresign{
			PresignId:  presignId,
			PidsString: pidString,
			Pids:       pids,
		}
	}

	m.lock.Lock()
	if ap, ok := m.available[pidString]; ok {
		ap = append(ap, arr...)
		m.available[pidString] = ap
	} else {
		m.available[pidString] = arr
	}
	m.lock.Unlock()
}

// GetAvailablePresigns returns a list of presigns with size batchSize for a list of parties. It
// immediately consumes the presign set (i.e. the set is longer available.) to avoid dpulicated
// usage of presign.
func (m *defaultAvailablePresigns) GetAvailablePresigns(batchSize int, n int, allPids map[string]*tss.PartyID) ([]string, []*tss.PartyID) {
	selectedPidstring := ""
	var selectedAps []*common.AvailablePresign

	m.lock.RLock()
	for pidString, apArr := range m.available {
		pids := strings.Split(pidString, ",")
		ok := true
		for _, pid := range pids {
			if _, found := allPids[pid]; !found {
				ok = false
				break
			}
		}

		if ok && len(apArr) >= batchSize {
			// We found this.
			selectedPidstring = pidString
			break
		}
	}
	m.lock.RUnlock()

	if selectedPidstring == "" {
		return []string{}, []*tss.PartyID{}
	}

	// 2. Remove the selected presigns from the available set.
	m.lock.Lock()
	if selectedPidstring != "" {
		apArr := m.available[selectedPidstring]

		if len(apArr) >= batchSize { // We check again here in case other routine has consume this apArr
			selectedAps = apArr[:batchSize]
			// Remove this available presigns from the list.
			m.available[selectedPidstring] = apArr[batchSize:]

			if len(m.available[selectedPidstring]) == 0 {
				delete(m.available, selectedPidstring)
			}
		} else {
			return []string{}, []*tss.PartyID{}
		}
	}

	m.lock.Unlock()

	// Get selected pids
	presignIds := make([]string, len(selectedAps))
	for i, ap := range selectedAps {
		presignIds[i] = ap.PresignId
	}

	pidStrings := strings.Split(selectedPidstring, ",")
	selectedPids := make([]*tss.PartyID, len(pidStrings))

	for i, pidString := range pidStrings {
		for _, p := range allPids {
			if p.Id == pidString {
				selectedPids[i] = p
				break
			}
		}
	}

	// Update the DB.
	err := m.db.UpdatePresignStatus(presignIds)
	if err != nil {
		log.Error("Cannot update presign status, err = ", err)
		return []string{}, []*tss.PartyID{}
	}

	return presignIds, selectedPids
}
