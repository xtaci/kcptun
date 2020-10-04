// Package prnt provides common functionality for code generators.
package prnt

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Generator provides convenience methods for code generators. In particular it
// provides fmt-like methods which print to an internal buffer. It also allows
// any errors to be stored so they can be checked at the end, rather than having
// error checks obscuring the code generation.
type Generator struct {
	buf bytes.Buffer
	err error
}

// Raw provides direct access to the underlying output stream.
func (g *Generator) Raw() io.Writer {
	return &g.buf
}

// Printf prints to the internal buffer.
func (g *Generator) Printf(format string, args ...interface{}) {
	if g.err != nil {
		return
	}
	_, err := fmt.Fprintf(&g.buf, format, args...)
	g.AddError(err)
}

// NL prints a new line.
func (g *Generator) NL() {
	g.Printf("\n")
}

// Comment writes comment lines prefixed with "// ".
func (g *Generator) Comment(lines ...string) {
	for _, line := range lines {
		line = strings.TrimSpace("// " + line)
		g.Printf("%s\n", line)
	}
}

// AddError records an error in code generation. The first non-nil error will
// prevent printing operations from writing anything else, and the error will be
// returned from Result().
func (g *Generator) AddError(err error) {
	if err != nil && g.err == nil {
		g.err = err
	}
}

// Result returns the printed bytes. If any error was recorded with AddError
// during code generation, the first such error will be returned here.
func (g *Generator) Result() ([]byte, error) {
	return g.buf.Bytes(), g.err
}
