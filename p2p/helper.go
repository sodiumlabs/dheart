package p2p

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

const (
	HeaderLength        = 4
	TimeoutReadPayload  = time.Second * 10
	TimeoutWritePayload = time.Second * 10
	MaxPayload          = 20000000 // 20M
)

func ReadStreamWithBuffer(stream network.Stream) ([]byte, error) {
	streamReader := bufio.NewReader(stream)
	lengthBytes := make([]byte, HeaderLength)
	n, err := io.ReadFull(streamReader, lengthBytes)
	if n != HeaderLength || err != nil {
		return nil, fmt.Errorf("error in read the message head %w", err)
	}
	length := binary.LittleEndian.Uint32(lengthBytes)
	if length > MaxPayload {
		return nil, fmt.Errorf("payload length:%d exceed max payload length:%d", length, MaxPayload)
	}
	dataBuf := make([]byte, length)
	n, err = io.ReadFull(streamReader, dataBuf)
	if uint32(n) != length || err != nil {
		return nil, fmt.Errorf("short read err(%w), we would like to read: %d, however we only read: %d", err, length, n)
	}
	return dataBuf, nil
}

func WriteStreamWithBuffer(msg []byte, stream network.Stream) error {
	length := uint32(len(msg))
	lengthBytes := make([]byte, HeaderLength)
	binary.LittleEndian.PutUint32(lengthBytes, length)

	streamWrite := bufio.NewWriter(stream)
	n, err := streamWrite.Write(lengthBytes)
	if n != HeaderLength || err != nil {
		return fmt.Errorf("fail to write head: %w", err)
	}
	n, err = streamWrite.Write(msg)
	if err != nil {
		return err
	}
	if uint32(n) != length {
		return fmt.Errorf("short write, we would like to write: %d, however we only write: %d", length, n)
	}
	err = streamWrite.Flush()
	if err != nil {
		// return fmt.Errorf("fail to flush stream: %w", err)
		return err
	}
	return nil
}
