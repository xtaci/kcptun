package memguard

import (
	"github.com/awnumar/memguard/core"
)

/* Enhancement: check for low memory locking limit and print warning?*/

/*
ScrambleBytes overwrites an arbitrary buffer with cryptographically-secure random bytes.
*/
func ScrambleBytes(buf []byte) {
	if err := core.Scramble(buf); err != nil {
		core.Panic(err)
	}
}

/*
WipeBytes overwrites an arbitrary buffer with zeroes.
*/
func WipeBytes(buf []byte) {
	core.Wipe(buf)
}

/*
Purge resets the session key to a fresh value and destroys all existing LockedBuffers. Existing Enclave objects will no longer be decryptable.
*/
func Purge() {
	core.Purge()
}

/*
SafePanic wipes all it can before calling panic(v).
*/
func SafePanic(v interface{}) {
	core.Panic(v)
}

/*
SafeExit destroys everything sensitive before exiting with a specified status code.
*/
func SafeExit(c int) {
	core.Exit(c)
}
