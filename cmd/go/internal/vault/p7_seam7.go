package vault

// p7_seam7.go — additional package-level test seams (design/TEST-SEAMS.10.md)
// used by the seam7 coverage tests. Each defaults to the real
// implementation and is swapped by a test (with t.Cleanup restore) to
// drive an otherwise-unreachable KDF/entropy/dispatch failure arm. None
// is API: unexported, wired at a single call site in this package, and
// behaviour with the defaults is identical to calling the underlying
// function directly.

import (
	"crypto/rand"
	"io"
	"runtime"
)

var (
	// p7scryptKey seams scryptKey so the scrypt-error and derived
	// bad-key-length arms (encryptBlob, aesGCMOpen, vaultMasterKEK
	// callers) are drivable; scrypt with the fixed vault params never
	// errors and always returns 32 bytes, so those arms need the seam.
	p7scryptKey = scryptKey

	// p7ioReadFull seams io.ReadFull in slotKEK; the HKDF reader cannot
	// error for a 32-byte read, so the error arm needs the seam.
	p7ioReadFull = io.ReadFull

	// p7boxRand seams the entropy source handed to box.GenerateKey /
	// box.SealAnonymous so the keypair-generation and sealed-box error
	// arms are drivable.
	p7boxRand io.Reader = rand.Reader

	// p7goosName seams runtime.GOOS at the keychain/wincred dispatch
	// points so the darwin/windows arms are drivable on linux.
	p7goosName = runtime.GOOS

	// p7loadStore seams LoadStore at the under-lock reload sites so the
	// "store vanished/became unreadable between the outer read and the
	// locked reload" arms are drivable.
	p7loadStore = LoadStore

	// p7randomID seams randomID at the envelope keyring's KeyringID mint
	// so its entropy-failure arm is drivable.
	p7randomID = randomID
)
