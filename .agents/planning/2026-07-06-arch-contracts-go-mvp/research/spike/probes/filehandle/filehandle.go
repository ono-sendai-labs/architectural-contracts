// Package filehandle separates the two things Capslock's builtin map conflates:
//   - Mint exercises AMBIENT AUTHORITY: os.Open turns an ambient path (a
//     designator the caller invented) into an *os.File capability.
//   - Consume exercises only a CAPABILITY it was GRANTED: it reads from an
//     *os.File handed to it, calling (*os.File).Read directly (no io.ReadAll).
//
// Under the object-capability model the design targets, Consume holds no ambient
// authority — the authority was spent by whoever opened the file. The spike uses
// this probe to show a capability-map override ("authority = minting, not use")
// that reclassifies the *os.File *methods* SAFE while keeping os.Open FILES.
package filehandle

import "os"

// Mint exercises ambient authority (path -> handle). Must remain FILES.
func Mint(path string) (*os.File, error) { return os.Open(path) }

// Consume exercises only the granted capability (reads a handed-in handle). Under
// the ocap override it holds no ambient authority.
func Consume(f *os.File) (int, error) {
	buf := make([]byte, 512)
	return f.Read(buf)
}
