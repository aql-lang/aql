// Test seam (design/TEST-SEAMS.10.md): nativeDefaultRegistry seams
// native.DefaultRegistry for the DEFAULT newRegistry implementation's
// own init-failure arm. newRegistry itself is already a test seam
// (swapped wholesale by TestStartRegistryError), which means its
// real body — including this error check — never runs under that
// test. This inner seam lets a test drive the default body's error
// arm directly, by calling newRegistry() itself rather than replacing
// it.

package repl

import "github.com/aql-lang/aql/lang/go/native"

var nativeDefaultRegistry = native.DefaultRegistry
