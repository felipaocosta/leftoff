//go:build !linux

package app

// lockHolderPID: elsewhere the holder is only known from the lock note.
func lockHolderPID(string) int { return 0 }
