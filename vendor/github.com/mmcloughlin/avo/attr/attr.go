// Package attr provides attributes for text and data sections.
package attr

import (
	"fmt"
	"math/bits"
	"strings"
)

// Attribute represents TEXT or DATA flags.
type Attribute uint16

// Reference: https://github.com/golang/go/blob/aafe257390cc9048e8b5df898fabd79a9e0d4c39/src/runtime/textflag.h#L11-L37
//
//	// Don't profile the marked routine. This flag is deprecated.
//	#define NOPROF	1
//	// It is ok for the linker to get multiple of these symbols. It will
//	// pick one of the duplicates to use.
//	#define DUPOK	2
//	// Don't insert stack check preamble.
//	#define NOSPLIT	4
//	// Put this data in a read-only section.
//	#define RODATA	8
//	// This data contains no pointers.
//	#define NOPTR	16
//	// This is a wrapper function and should not count as disabling 'recover'.
//	#define WRAPPER 32
//	// This function uses its incoming context register.
//	#define NEEDCTXT 64
//	// Allocate a word of thread local storage and store the offset from the
//	// thread local base to the thread local storage in this variable.
//	#define TLSBSS	256
//	// Do not insert instructions to allocate a stack frame for this function.
//	// Only valid on functions that declare a frame size of 0.
//	// TODO(mwhudson): only implemented for ppc64x at present.
//	#define NOFRAME 512
//	// Function can call reflect.Type.Method or reflect.Type.MethodByName.
//	#define REFLECTMETHOD 1024
//	// Function is the top of the call stack. Call stack unwinders should stop
//	// at this function.
//	#define TOPFRAME 2048
//
const (
	NOPROF Attribute = 1 << iota
	DUPOK
	NOSPLIT
	RODATA
	NOPTR
	WRAPPER
	NEEDCTXT
	_
	TLSBSS
	NOFRAME
	REFLECTMETHOD
	TOPFRAME
)

// Asm returns a representation of the attributes in assembly syntax. This may use macros from "textflags.h"; see ContainsTextFlags() to determine if this header is required.
func (a Attribute) Asm() string {
	parts, rest := a.split()
	if len(parts) == 0 || rest != 0 {
		parts = append(parts, fmt.Sprintf("%d", rest))
	}
	return strings.Join(parts, "|")
}

// ContainsTextFlags returns whether the Asm() representation requires macros in "textflags.h".
func (a Attribute) ContainsTextFlags() bool {
	flags, _ := a.split()
	return len(flags) > 0
}

// split splits a into known flags and any remaining bits.
func (a Attribute) split() ([]string, Attribute) {
	var flags []string
	var rest Attribute
	for a != 0 {
		i := uint(bits.TrailingZeros16(uint16(a)))
		bit := Attribute(1) << i
		if flag := attrname[bit]; flag != "" {
			flags = append(flags, flag)
		} else {
			rest |= bit
		}
		a ^= bit
	}
	return flags, rest
}

var attrname = map[Attribute]string{
	NOPROF:        "NOPROF",
	DUPOK:         "DUPOK",
	NOSPLIT:       "NOSPLIT",
	RODATA:        "RODATA",
	NOPTR:         "NOPTR",
	WRAPPER:       "WRAPPER",
	NEEDCTXT:      "NEEDCTXT",
	TLSBSS:        "TLSBSS",
	NOFRAME:       "NOFRAME",
	REFLECTMETHOD: "REFLECTMETHOD",
	TOPFRAME:      "TOPFRAME",
}
