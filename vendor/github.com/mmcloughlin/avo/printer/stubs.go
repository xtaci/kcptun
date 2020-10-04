package printer

import (
	"github.com/mmcloughlin/avo/internal/prnt"
	"github.com/mmcloughlin/avo/ir"
)

type stubs struct {
	cfg Config
	prnt.Generator
}

// NewStubs constructs a printer for writing stub function declarations.
func NewStubs(cfg Config) Printer {
	return &stubs{cfg: cfg}
}

func (s *stubs) Print(f *ir.File) ([]byte, error) {
	s.Comment(s.cfg.GeneratedWarning())

	if len(f.Constraints) > 0 {
		s.NL()
		s.Printf(f.Constraints.GoString())
	}

	s.NL()
	s.Printf("package %s\n", s.cfg.Pkg)
	for _, fn := range f.Functions() {
		s.NL()
		s.Comment(fn.Doc...)
		for _, pragma := range fn.Pragmas {
			s.pragma(pragma)
		}
		s.Printf("%s\n", fn.Stub())
	}
	return s.Result()
}

func (s *stubs) pragma(p ir.Pragma) {
	s.Printf("//go:%s", p.Directive)
	for _, arg := range p.Arguments {
		s.Printf(" %s", arg)
	}
	s.NL()
}
