package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sisu-network/lib/log"
	"github.com/sodiumlabs/tss-lib/tss"
)

func NewTssMessage(from, to, workId string, msgs []tss.Message, round string) (*TssMessage, error) {
	// Serialize tss messages
	updateMessages := make([]*UpdateMessage, len(msgs))

	for i, msg := range msgs {
		msgsBytes, routing, err := msg.WireBytes()
		if err != nil {
			log.Error("error when get wire bytes from tss message: ", err)
			return nil, err
		}

		serialized, err := json.Marshal(routing)
		if err != nil {
			log.Error("error when serialized routing info: ", err)
			return nil, err
		}

		updateMessages[i] = &UpdateMessage{
			Data:                     msgsBytes,
			SerializedMessageRouting: serialized,
			Round:                    round,
		}
	}

	msg := baseMessage(TssMessage_UPDATE_MESSAGES, from, to, workId)
	msg.UpdateMessages = updateMessages

	return msg, nil
}

func NewAvailabilityRequestMessage(from, to, workId string) *TssMessage {
	msg := baseMessage(TssMessage_AVAILABILITY_REQUEST, from, to, workId)
	return msg
}

func NewAvailabilityResponseMessage(from, to, workId string, answer AvailabilityResponseMessage_ANSWER, maxJob int) *TssMessage {
	msg := baseMessage(TssMessage_AVAILABILITY_RESPONSE, from, to, workId)
	msg.AvailabilityResponseMessage = &AvailabilityResponseMessage{
		Answer: AvailabilityResponseMessage_YES,
		MaxJob: int32(maxJob),
	}

	return msg
}

func NewPreExecOutputMessage(from, to, workId string, success bool, presignIds []string, pids []*tss.PartyID) *TssMessage {
	msg := baseMessage(TssMessage_PRE_EXEC_OUTPUT, from, to, workId)

	// get all pid strings
	s := make([]string, len(pids))
	for i, p := range pids {
		s[i] = p.Id
	}

	msg.PreExecOutputMessage = &PreExecOutputMessage{
		Success:    success,
		Pids:       s,
		PresignIds: presignIds,
	}

	return msg
}

func NewRequestMessage(from, to, workId, msgKey string) *TssMessage {
	msg := baseMessage(TssMessage_ASK_MESSAGE_REQUEST, from, to, workId)
	msg.AskRequestMessage = &AskRequestMessage{
		MsgKey: msgKey,
	}

	return msg
}

func baseMessage(typez TssMessage_Type, from, to, workId string) *TssMessage {
	return &TssMessage{
		Type:   typez,
		From:   from,
		To:     to,
		WorkId: workId,
	}
}

func (msg *TssMessage) IsBroadcast() bool {
	return msg.To == ""
}

func (msg *TssMessage) GetMessageKey() (string, error) {
	if len(msg.UpdateMessages) == 0 {
		err := fmt.Errorf("empty update messages")
		log.Error(err)
		return "", err
	}

	return GetMessageKey(msg.WorkId, msg.From, msg.To, msg.UpdateMessages[0].Round), nil
}

func (msg *TssMessage) IsSigningMessage() bool {
	return msg.Type == TssMessage_UPDATE_MESSAGES && len(msg.UpdateMessages) > 0 &&
		msg.UpdateMessages[0].Round == "SignRound1Message"
}

func GetMessageKey(workdId, from, to, tssMsgType string) string {
	return fmt.Sprintf("%s__%s__%s__%s", workdId, from, to, tssMsgType)
}

// ExtractMessageKey returns workId, from, to, tssMsgType
func ExtractMessageKey(msgKey string) ([]string, error) {
	if len(msgKey) == 0 {
		return nil, nil
	}
	keys := strings.Split(msgKey, "__")
	if len(keys) < 4 {
		err := fmt.Errorf("not enough keys. msgKey = %s", msgKey)
		log.Error(err)
		return nil, err
	}

	return keys, nil
}

func GetPreworkSelectionMsgType() map[TssMessage_Type]bool {
	return map[TssMessage_Type]bool{
		TssMessage_AVAILABILITY_REQUEST:  true,
		TssMessage_AVAILABILITY_RESPONSE: true,
		TssMessage_PRE_EXEC_OUTPUT:       true,
	}
}

func GetUpdateMessageType() map[TssMessage_Type]bool {
	return map[TssMessage_Type]bool{
		TssMessage_UPDATE_MESSAGES: true,
	}
}
