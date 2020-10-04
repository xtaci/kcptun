package operand

import "fmt"

// Constant represents a constant literal.
type Constant interface {
	Op
	Bytes() int
	constant()
}

//go:generate go run make_const.go -output zconst.go

// String is a string constant.
type String string

// Asm returns an assembly syntax representation of the string s.
func (s String) Asm() string { return fmt.Sprintf("$%q", s) }

// Bytes returns the length of s.
func (s String) Bytes() int { return len(s) }

func (s String) constant() {}

// Imm returns an unsigned integer constant with size guessed from x.
func Imm(x uint64) Constant {
	switch {
	case uint64(uint8(x)) == x:
		return U8(x)
	case uint64(uint16(x)) == x:
		return U16(x)
	case uint64(uint32(x)) == x:
		return U32(x)
	}
	return U64(x)
}
