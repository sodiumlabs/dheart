package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/sisu-network/lib/log"
	"github.com/sodiumlabs/dheart/core/config"
	"github.com/sodiumlabs/dheart/core/message"
	"github.com/sodiumlabs/dheart/db"
	"github.com/sodiumlabs/dheart/types/common"
	commonTypes "github.com/sodiumlabs/dheart/types/common"
	"github.com/sodiumlabs/dheart/worker/components"
	"github.com/sodiumlabs/dheart/worker/helper"
	"github.com/sodiumlabs/dheart/worker/interfaces"
	"github.com/sodiumlabs/dheart/worker/types"
	wTypes "github.com/sodiumlabs/dheart/worker/types"
	eckeygen "github.com/sodiumlabs/tss-lib/ecdsa/keygen"
	ecsigning "github.com/sodiumlabs/tss-lib/ecdsa/signing"
	"github.com/sodiumlabs/tss-lib/tss"
	"go.uber.org/atomic"
)

type ExecutionResult struct {
	Success bool
	Request *types.WorkRequest

	JobResults []*JobResult
}

type WorkerExecutor struct {
	///////////////////////
	// Immutable data
	///////////////////////

	request    *types.WorkRequest
	workType   wTypes.WorkType
	myPid      *tss.PartyID
	pIDs       []*tss.PartyID // This pids and pIDsMap do not have lock as they should be immutable
	pIDsMap    map[string]*tss.PartyID
	dispatcher interfaces.MessageDispatcher
	db         db.Database
	cfg        config.TimeoutConfig

	// ECDSA Input
	ecKeygenInput   *eckeygen.LocalPreParams
	ecPresignOutput []*ecsigning.SignatureData_OneRoundData

	callback func(*WorkerExecutor, ExecutionResult)

	///////////////////////
	// Mutable data
	///////////////////////

	// A map between of rounds and list of messages that have been produced. The size of the list
	// is the same as batchSize.
	jobOutput     map[string][]tss.Message
	jobOutputLock *sync.RWMutex

	jobResults []*JobResult

	finalOutputLock *sync.RWMutex
	isStopped       atomic.Bool

	jobs           []*Job
	jobsLock       *sync.RWMutex
	messageMonitor components.MessageMonitor
}

func NewWorkerExecutor(
	request *types.WorkRequest,
	workType wTypes.WorkType,
	myPid *tss.PartyID,
	pids []*tss.PartyID,
	dispatcher interfaces.MessageDispatcher,
	db db.Database,
	ecPresignOutput []*ecsigning.SignatureData_OneRoundData,
	callback func(*WorkerExecutor, ExecutionResult),
	cfg config.TimeoutConfig,
) *WorkerExecutor {
	pIDsMap := make(map[string]*tss.PartyID)
	for _, pid := range pids {
		pIDsMap[pid.Id] = pid
	}

	if ecPresignOutput == nil {
		ecPresignOutput = make([]*ecsigning.SignatureData_OneRoundData, request.BatchSize)
	}

	// Make a copy of my pid and assign correct index to it to avoid race condition.
	copy := tss.NewPartyID(myPid.Id, myPid.Moniker, myPid.KeyInt())
	// Assign the correct index for our pid.
	for _, p := range pids {
		if myPid.Id == p.Id {
			copy.Index = p.Index
		}
	}

	return &WorkerExecutor{
		request:         request,
		workType:        workType,
		myPid:           copy,
		pIDs:            pids,
		pIDsMap:         pIDsMap,
		dispatcher:      dispatcher,
		db:              db,
		callback:        callback,
		ecPresignOutput: ecPresignOutput,
		jobsLock:        &sync.RWMutex{},
		jobOutputLock:   &sync.RWMutex{},
		finalOutputLock: &sync.RWMutex{},
		jobResults:      make([]*JobResult, request.BatchSize),
		jobOutput:       make(map[string][]tss.Message),
		isStopped:       *atomic.NewBool(false),
		cfg:             cfg,
	}
}

func (w *WorkerExecutor) Init() (err error) {
	if w.workType == wTypes.EcKeygen {
		if w.request.EcKeygenInput == nil {
			err = w.loadPreparams()
		} else {
			w.ecKeygenInput = w.request.EcKeygenInput
		}
	}

	w.messageMonitor = components.NewMessageMonitor(w.myPid, w.workType, w, w.pIDsMap, w.cfg.MonitorMessageTimeout)
	go w.messageMonitor.Start()

	p2pCtx := tss.NewPeerContext(w.pIDs)
	params := tss.NewParameters(p2pCtx, w.myPid, len(w.pIDs), w.request.Threshold)
	batchSize := w.request.BatchSize
	jobs := make([]*Job, batchSize)
	log.Infof("%s WorkerExecutor WorkType = %s", w.myPid.Id, w.workType)

	workId := w.request.WorkId
	// Creates all jobs
	for i := range jobs {
		switch w.workType {
		// Ecdsa
		case wTypes.EcKeygen:
			jobs[i] = NewEcKeygenJob(workId, i, w.pIDs, params, w.ecKeygenInput, w, w.cfg.KeygenJobTimeout)

		case wTypes.EcSigning:
			var presignOutput *ecsigning.SignatureData_OneRoundData
			if w.ecPresignOutput == nil {
				presignOutput = nil
			} else {
				presignOutput = w.ecPresignOutput[i]
			}
			jobs[i] = NewEcSigningJob(workId, i, w.pIDs, params, []byte(w.request.Messages[i]),
				*w.request.EcSigningInput, presignOutput, w, w.cfg.SigningJobTimeout, w.request.Chains[0])

		// Eddsa
		case wTypes.EdKeygen:
			jobs[i] = NewEdKeygenJob(workId, i, w.pIDs, params, w, w.cfg.KeygenJobTimeout)

		case wTypes.EdSigning:
			jobs[i] = NewEdSigningJob(workId, i, w.pIDs, params, []byte(w.request.Messages[i]), *w.request.EdSigningInput, w, w.cfg.SigningJobTimeout)

		default:
			// If job type is not correct, kill the whole worker.
			w.broadcastResult(ExecutionResult{
				Success: false,
			})

			log.Errorf("unknown work type %d", w.workType)
			return
		}
	}

	w.jobsLock.Lock()
	w.jobs = jobs
	w.jobsLock.Unlock()

	for _, job := range jobs {
		if err := job.Start(); err != nil {
			log.Critical("error when starting job, err = ", err)
			// If job cannot start, kill the whole worker.
			go w.broadcastResult(ExecutionResult{
				Success: false,
			})

			break
		}
	}

	return nil
}

func (w *WorkerExecutor) Run(cachedMsgs []*commonTypes.TssMessage) {
	if w.isStopped.Load() {
		return
	}

	log.Info(w.myPid.Id, " ", w.request.WorkId, " ", w.workType, " Cache size =", len(cachedMsgs))

	for _, msg := range cachedMsgs {
		if msg.Type == common.TssMessage_UPDATE_MESSAGES {
			if err := w.ProcessUpdateMessage(msg); err != nil {
				// Message can be corrupted or from bad actor, continue to execute.
				log.Error("Error when processing new message", err)
			}
		}
	}
}

func (w *WorkerExecutor) loadPreparams() error {
	// Check if we have generated preparams
	var err error
	preparams, err := w.db.LoadPreparams()
	if err == nil {
		log.Info("Preparams found")
		w.ecKeygenInput = preparams
	} else {
		log.Error("Failed to get preparams, err =", err)
		return err
	}

	return nil
}

// Called when there is a new message from tss-lib. We want this callback to be open even when the
// executor might stop. Other validator nodes are dependent on our messages and we should keep
// producing and sending tss update messages to other nodes.
func (w *WorkerExecutor) OnJobMessage(job *Job, msg tss.Message) {
	// Update the list of completed jobs for current round (in the message)
	msgKey := msg.Type()
	if !msg.IsBroadcast() {
		msgKey = msgKey + "-" + msg.GetTo()[0].Id
	}

	// Update the list of finished jobs for msgKey
	w.jobOutputLock.Lock()
	list := w.jobOutput[msgKey]
	if list == nil {
		list = make([]tss.Message, w.request.BatchSize)
	}
	list[job.index] = msg
	w.jobOutput[msgKey] = list
	// Count how many job that have completed.
	count := w.getCompletedJobCount(list)
	w.jobOutputLock.Unlock()

	if count == w.request.BatchSize {
		// We have completed all jobs for current round. Send the list to the dispatcher. Move the worker to next round.
		dest := msg.GetTo()
		to := ""
		if dest != nil {
			to = dest[0].Id
		}

		tssMsg, err := common.NewTssMessage(w.myPid.Id, to, w.request.WorkId, list, msg.Type())
		if err != nil {
			log.Critical("Cannot build TSS message, err", err)
			return
		}

		if dest == nil {
			w.dispatcher.BroadcastMessage(w.pIDs, tssMsg)
		} else {
			w.dispatcher.UnicastMessage(dest[0], tssMsg)
		}
	}
}

// Implements JobCallback
func (w *WorkerExecutor) OnJobResult(job *Job, result JobResult) {
	if w.isStopped.Load() {
		return
	}

	w.finalOutputLock.Lock()
	w.jobResults[job.index] = &result
	count := 0
	hasFailure := false
	for _, result := range w.jobResults {
		if result != nil {
			count++
			if !result.Success {
				hasFailure = true
			}
		}
	}
	w.finalOutputLock.Unlock()

	if count == w.request.BatchSize {
		if hasFailure {
			// If any of the job fails, this is considered to be a failure.
			w.broadcastResult(ExecutionResult{
				Success: false,
			})
		} else {
			w.broadcastResult(ExecutionResult{
				Success:    true,
				JobResults: w.jobResults,
			})
		}
	}
}

// Implements MessageMonitorCallback
func (w *WorkerExecutor) OnMissingMesssageDetected(m map[string][]string) {
	if w.isStopped.Load() {
		return
	}

	workId := w.request.WorkId
	// We have found missing messages
	for pid, msgTypes := range m {
		for _, msgType := range msgTypes {
			if message.IsBroadcastMessage(msgType) {
				msgKey := common.GetMessageKey(workId, pid, "", msgType)
				msg := common.NewRequestMessage(w.myPid.Id, "", workId, msgKey)

				w.dispatcher.BroadcastMessage(w.pIDs, msg)
			} else {
				msgKey := common.GetMessageKey(workId, pid, w.myPid.Id, msgType)
				msg := common.NewRequestMessage(w.myPid.Id, pid, workId, msgKey)

				w.dispatcher.UnicastMessage(w.pIDsMap[pid], msg)
			}
		}
	}
}

func (w *WorkerExecutor) ProcessUpdateMessage(tssMsg *commonTypes.TssMessage) error {
	if w.isStopped.Load() {
		return nil
	}

	// Do all message validation first before processing.
	// TODO: Add more validation here.
	msgs := make([]tss.ParsedMessage, w.request.BatchSize)
	// Now update all messages
	w.jobsLock.RLock()
	jobs := w.jobs
	w.jobsLock.RUnlock()

	fromString := tssMsg.From
	from := helper.GetPidFromString(fromString, w.pIDs)
	if from == nil {
		return errors.New("sender is nil")
	}

	for i := range jobs {
		updateMessage := tssMsg.UpdateMessages[i]
		msg, err := tss.ParseWireMessage(updateMessage.Data, from, tssMsg.IsBroadcast())
		if err != nil {
			return fmt.Errorf("error when parsing wire message %w", err)
		}

		msgRouting := tss.MessageRouting{}
		if err := json.Unmarshal(updateMessage.SerializedMessageRouting, &msgRouting); err != nil {
			return fmt.Errorf("error when unmarshal message routing %w", err)
		}

		msgs[i] = msg
	}

	// Update the message monitor
	w.messageMonitor.NewMessageReceived(msgs[0], from)

	for i, j := range jobs {
		go func(jobIndex int, job *Job) {
			_, err := message.GetMsgRound(msgs[jobIndex].Content())
			if err != nil {
				log.Error("error when getting round %w", err)
				return
			}

			if err := job.processMessage(msgs[jobIndex]); err != nil {
				log.Error("worker: cannot process message, err = ", err)

				w.broadcastResult(ExecutionResult{
					Success: false,
				})
				return
			}
		}(i, j)
	}

	return nil
}

func (w *WorkerExecutor) Stop() {
	w.isStopped.Store(true)
}

func (w *WorkerExecutor) OnJobTimeout() {
	w.broadcastResult(ExecutionResult{
		Success: false,
	})
}

func (w *WorkerExecutor) broadcastResult(result ExecutionResult) {
	w.jobsLock.Lock()
	defer w.jobsLock.Unlock()

	if !w.isStopped.Load() {
		w.isStopped.Store(true)

		go w.callback(w, result) // Make the callback in separate go routine to avoid expensive blocking.
		if w.messageMonitor != nil {
			go w.messageMonitor.Stop()
		}
	}
}

func (w *WorkerExecutor) getCompletedJobCount(list []tss.Message) int {
	count := 0
	for _, item := range list {
		if item != nil {
			count++
		}
	}

	return count
}
