package generic

import (
	"io"

	"github.com/xtaci/qpp"
)

// QPPPort implements io.ReadWriteCloser interface for Quantum Permutation Pads
type QPPPort struct {
	underlying io.ReadWriteCloser // io.Writer is not enough, we need to close the underlying writer as well

	qpp   *qpp.QuantumPermutationPad
	wprng *qpp.Rand
	rprng *qpp.Rand
}

func NewQPPPort(underlying io.ReadWriteCloser, qpp *qpp.QuantumPermutationPad, seed []byte) *QPPPort {
	wprng := qpp.CreatePRNG(seed)
	rprng := qpp.CreatePRNG(seed)
	return &QPPPort{underlying, qpp, wprng, rprng}
}

func (port *QPPPort) Read(p []byte) (n int, err error) {
	n, err = port.underlying.Read(p)
	port.qpp.DecryptWithPRNG(p[:n], port.rprng)
	return
}

func (r *QPPPort) Write(p []byte) (n int, err error) {
	r.qpp.EncryptWithPRNG(p, r.wprng)
	return r.underlying.Write(p)
}

func (r *QPPPort) Close() error {
	return r.underlying.Close()
}
