package smux

import (
	"encoding/binary"
	"fmt"
)

const ( // cmds
	// protocol version 1:
	cmdSYN byte = iota // stream open
	cmdFIN             // stream close, a.k.a EOF mark
	cmdPSH             // data push
	cmdNOP             // no operation

	// protocol version 2 extra commands
	// notify bytes consumed by remote peer-end
	cmdUPD
)

const (
	// data size of cmdUPD, format:
	// |4B data consumed(ACK)| 4B window size(WINDOW) |
	szCmdUPD = 8
)

const (
	// initial peer window guess, a slow-start
	initialPeerWindow = 262144
)

const (
	sizeOfVer    = 1
	sizeOfCmd    = 1
	sizeOfLength = 2
	sizeOfSid    = 4
	headerSize   = sizeOfVer + sizeOfCmd + sizeOfSid + sizeOfLength
)

// Frame defines a packet from or to be multiplexed into a single connection
type Frame struct {
	ver  byte
	cmd  byte
	sid  uint32
	data []byte
}

func newFrame(version byte, cmd byte, sid uint32) Frame {
	return Frame{ver: version, cmd: cmd, sid: sid}
}

type rawHeader [headerSize]byte

func (h rawHeader) Version() byte {
	return h[0]
}

func (h rawHeader) Cmd() byte {
	return h[1]
}

func (h rawHeader) Length() uint16 {
	return binary.LittleEndian.Uint16(h[2:])
}

func (h rawHeader) StreamID() uint32 {
	return binary.LittleEndian.Uint32(h[4:])
}

func (h rawHeader) String() string {
	return fmt.Sprintf("Version:%d Cmd:%d StreamID:%d Length:%d",
		h.Version(), h.Cmd(), h.StreamID(), h.Length())
}

type updHeader [szCmdUPD]byte

func (h updHeader) Consumed() uint32 {
	return binary.LittleEndian.Uint32(h[:])
}
func (h updHeader) Window() uint32 {
	return binary.LittleEndian.Uint32(h[4:])
}
