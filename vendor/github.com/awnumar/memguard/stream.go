package memguard

import (
	"container/list"
	"io"
	"os"
	"sync"

	"github.com/awnumar/memguard/core"
)

var (
	// StreamChunkSize is the maximum amount of data that is locked into memory at a time.
	// If you get error allocating memory, increase your system's mlock limits.
	// Use 'ulimit -l' to see mlock limit on unix systems.
	StreamChunkSize = c
	c               = os.Getpagesize() * 4
)

type queue struct {
	*list.List
}

// add data to back of queue
func (q *queue) join(e *Enclave) {
	q.PushBack(e)
}

// add data to front of queue
func (q *queue) push(e *Enclave) {
	q.PushFront(e)
}

// pop data off front of queue
// returns nil if queue is empty
func (q *queue) pop() *Enclave {
	e := q.Front() // get element at front of queue
	if e == nil {
		return nil // no data
	}
	q.Remove(e)               // success => remove value
	return e.Value.(*Enclave) // unwrap and return (potential panic)
}

/*
Stream is an in-memory encrypted container implementing the reader and writer interfaces.

It is most useful when you need to store lots of data in memory and are able to work on it in chunks.
*/
type Stream struct {
	sync.Mutex
	*queue
}

// NewStream initialises a new empty Stream object.
func NewStream() *Stream {
	return &Stream{queue: &queue{List: list.New()}}
}

/*
Write encrypts and writes some given data to a Stream object.

The data is broken down into chunks and added to the stream in order. The last thing to be written to the stream is the last thing that will be read back.
*/
func (s *Stream) Write(data []byte) (int, error) {
	s.Lock()
	defer s.Unlock()

	for i := 0; i < len(data); i += c {
		if i+c > len(data) {
			s.join(NewEnclave(data[len(data)-(len(data)%c):]))
		} else {
			s.join(NewEnclave(data[i : i+c]))
		}
	}
	return len(data), nil
}

/*
Read decrypts and places some data from a Stream object into a provided buffer.

If there is no data, the call will return an io.EOF error. If the caller provides a buffer
that is too small to hold the next chunk of data, the remaining bytes are re-encrypted and
added to the front of the queue to be returned in the next call.

To be performant, have
*/
func (s *Stream) Read(buf []byte) (int, error) {
	s.Lock()
	defer s.Unlock()

	// Grab the next chunk of data from the stream.
	b, err := s.next()
	if err != nil {
		return 0, err
	}
	defer b.Destroy()

	// Copy the contents into the given buffer.
	core.Copy(buf, b.Bytes())

	// Check if there is data left over.
	if len(buf) < b.Size() {
		// Re-encrypt it and push onto the front of the list.
		c := NewBuffer(b.Size() - len(buf))
		c.Copy(b.Bytes()[len(buf):])
		s.push(c.Seal())
		return len(buf), nil
	}

	// Not enough data or perfect amount of data.
	// Either way we copied the entire buffer.
	return b.Size(), nil
}

// Size returns the number of bytes of data currently stored within a Stream object.
func (s *Stream) Size() int {
	s.Lock()
	defer s.Unlock()

	var n int
	for e := s.Front(); e != nil; e = e.Next() {
		n += e.Value.(*Enclave).Size()
	}
	return n
}

// Next grabs the next chunk of data from the Stream and returns it decrypted inside a LockedBuffer. Any error from the stream is forwarded.
func (s *Stream) Next() (*LockedBuffer, error) {
	s.Lock()
	defer s.Unlock()

	return s.next()
}

// does not acquire mutex lock
func (s *Stream) next() (*LockedBuffer, error) {
	// Pop data from the front of the list.
	e := s.pop()
	if e == nil {
		return newNullBuffer(), io.EOF
	}

	// Decrypt the data into a guarded allocation.
	b, err := e.Open()
	if err != nil {
		return newNullBuffer(), err
	}
	return b, nil
}

// Flush reads all of the data from a Stream and returns it inside a LockedBuffer. If an error is encountered before all the data could be read, it is returned along with any data read up until that point.
func (s *Stream) Flush() (*LockedBuffer, error) {
	return NewBufferFromEntireReader(s)
}
