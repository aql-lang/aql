package vault

// w8_seam8.go — wave-8 package-level test seams (design/TEST-SEAMS.10.md).
//
// Each variable defaults to the real implementation and is unexported;
// production behaviour with the defaults is identical to calling the
// underlying function directly (a `var x = f` alias adds no statements and
// no runtime check). Tests swap them (with a t.Cleanup restore) to drive
// otherwise-unreachable defensive arms:
//
//   - w8requireScope seams the exec read-scope gate. Every real session tier
//     (and a legacy session) satisfies OpRead, so exec's permission-denied
//     arm is reachable only through the seam (mirrors export's c7requireScope).
//   - w8findAliasRef seams the alias-reference resolution that precedes the
//     under-lock mutate in `expiry set/clear`, `grant`, and `rotate`. The
//     mutate re-checks that the resolved alias still exists in the freshly
//     reloaded store; that guard fires only when the alias vanished between
//     the resolve and the locked re-read (a concurrent removal). The seam
//     makes the resolver hand back a name the reloaded store lacks, so the
//     guard is drivable without a second writer.
//   - w8writeSecret / w8mutateStore seam the keyring-write and store-mutate
//     steps (WriteSecret, and add's write step for w8writeSecret) so their
//     failure arms are drivable — both succeed on a healthy vault. A test may
//     also use w8writeSecret (or w8findAliasRef / c7sess*Value) to land a
//     concurrent change (a lock, a colliding alias) between a command's
//     pre-check and its under-lock re-check, driving the guards that defend
//     the mutate against a racing writer.
var (
	w8requireScope = requireScope
	w8findAliasRef = findAliasRef
	w8writeSecret  = writeSecret
	w8mutateStore  = mutateStore
)
