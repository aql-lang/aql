package eng

import "fmt"

// Word extensions — the open-words merge (design/OPEN-WORDS.0.md).
//
// `def <word> fn […]` on a word that carries at least one LOCKED
// signature (a native / host registration, or a module-wrapper
// rebinding that inherited locked inner sigs) does not replace the
// word: it constructs a WORD CLONE — the base word's complete
// signature list plus the merge — and binds it through the ordinary
// DefTable shadow stack. Scoping, `undef`, closure capture and
// sub-engine inheritance all fall out of the existing def machinery;
// Registry.Lookup treats a clone as occluding deeper entries for the
// name (the clone already contains them).
//
// Merge rules (§2.1):
//   - a signature whose argument-type tuple exactly matches an
//     existing UNLOCKED signature REPLACES it;
//   - a tuple matching a LOCKED signature is an error
//     ([aql/locked_signature]) — locked sigs can never be replaced;
//   - any other signature APPENDS. Locked sigs keep first position in
//     match order via CompareSignatures' locked-first key.

// sealedWords are the words the engine special-cases BY NAME, where a
// shadow binding would break the identity the kernel relies on — so
// they cannot be def-merged at all (§2.3). The inventory comes from
// the §4.5 audit of name-keyed special cases in the kernel:
//   - "def"  — engine.go::bindsReferent, the pending-forward hints,
//     and macro_expand.go all treat the name as a reliable identity.
//   - "make" — execMatch's autoEvalMap gating keys on match.Name.
//   - "word" — carrier.go's splice-paren expansion keys on the name.
//
// The literals true/false/none/inf/nan are name-cased too but are not
// registered words (reservedLiterals guards them separately).
var sealedWords = map[string]bool{
	"def":  true,
	"make": true,
	"word": true,
}

// IsSealedWord reports whether name is sealed against def-merging —
// the strong tier above locked signatures (§2.3).
func IsSealedWord(name string) bool { return sealedWords[name] }

// hasLockedSig reports whether any signature in the slice is locked.
func hasLockedSig(sigs []Signature) bool {
	for i := range sigs {
		if sigs[i].Locked {
			return true
		}
	}
	return false
}

// HasLockedSigs reports whether the definition carries at least one
// locked signature — the merge trigger for `def <word> fn […]` (§4.1):
// locked-bearing words merge, plain user fns keep whole-replacement
// shadowing (the REPL/iterate idiom).
func HasLockedSigs(fn *FnDefInfo) bool {
	return fn != nil && hasLockedSig(fn.Signatures)
}

// IsWordExtension reports whether v is a word-extension clone and
// returns its FnDefInfo. The named-helper protocol for the provenance
// marker (never probe the Extends field inline) — recognised by the
// import machinery for the export transplant (§2.4).
func IsWordExtension(v Value) (FnDefInfo, bool) {
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok || fnDef.Extends == "" {
		return FnDefInfo{}, false
	}
	return fnDef, true
}

// sigTupleEqual reports whether two signatures declare the SAME
// argument tuple — same arity, same declared type at every position,
// and matching value patterns (both absent, or both present and
// canon-identical). This is the exact-match rule the merge uses to
// decide replace-vs-append; it is deliberately stricter than overlap.
func sigTupleEqual(a, b *Signature) bool {
	if a.TotalArgs() != b.TotalArgs() {
		return false
	}
	for i := 0; i < a.TotalArgs(); i++ {
		at, bt := sigArgType(a, i), sigArgType(b, i)
		if at == nil {
			at = TAny
		}
		if bt == nil {
			bt = TAny
		}
		if !at.Equal(bt) {
			return false
		}
		ap, aok := sigPattern(a, i)
		bp, bok := sigPattern(b, i)
		if aok != bok {
			return false
		}
		if aok && CanonValue(ap) != CanonValue(bp) {
			return false
		}
	}
	return true
}

// mergeExtensionSigs merges incoming signatures into a copy of base's
// dispatch list (fallbacks dropped — Lookup re-synthesises one), per
// the §2.1 rules. origin tags each installed signature's provenance
// ("" for a direct user def; the module ref for a transplant). It
// reports whether anything changed — an entirely idempotent transplant
// (diamond re-import) installs nothing.
func mergeExtensionSigs(r *Registry, name string, base *FnDefInfo, incoming []Signature, origin string) ([]Signature, bool, error) {
	merged := make([]Signature, 0, len(base.Signatures)+len(incoming))
	for i := range base.Signatures {
		if base.Signatures[i].Fallback {
			continue
		}
		merged = append(merged, base.Signatures[i])
	}
	changed := false
	for i := range incoming {
		ns := incoming[i]
		ns.Locked = false
		ns.Origin = origin
		at := -1
		for j := range merged {
			if sigTupleEqual(&merged[j], &ns) {
				at = j
				break
			}
		}
		if at < 0 {
			merged = append(merged, ns)
			changed = true
			continue
		}
		if merged[at].Locked {
			return nil, false, r.AqlErrorHint("locked_signature",
				fmt.Sprintf("def %s: signature %s matches a locked built-in signature and cannot replace it",
					name, sigTupleString(&ns)),
				"def",
				"locked signatures are frozen; add a signature with a different argument-type tuple, or define a new word")
		}
		if origin != "" {
			// Transplant collision policy (§4.4): the same unlocked tuple
			// arriving from a DIFFERENT module than the one that installed
			// it is a loud error — two files that never mention each other
			// must not silently shadow. Identical provenance (diamond
			// re-import) is idempotent and quiet. A direct user def
			// (existing Origin == "") still shadows freely.
			if merged[at].Origin == origin {
				continue
			}
			if merged[at].Origin != "" {
				return nil, false, r.AqlErrorHint("extend_conflict",
					fmt.Sprintf("import: modules %q and %q both extend %s with signature %s",
						merged[at].Origin, origin, name, sigTupleString(&ns)),
					"import",
					"import only one of the extensions, or firewall one module behind an inline module that re-exports just what you need")
			}
		}
		merged[at] = ns
		changed = true
	}
	return merged, changed, nil
}

// isKernelType reports whether t is a KERNEL-declared builtin — a type
// core owns and the parser can produce (FixedID inside the eng ranges,
// below 1000). External-builtin domain types (Date, Matrix, Fetch,
// Timeout — FixedID >= 1000, registered via RegisterExternalBuiltin by
// their owning module) and runtime-minted user types are NOT kernel
// types. nil (an untyped Any slot) counts as kernel.
func isKernelType(t *Type) bool {
	return t == nil || (t.Origin == OriginBuiltin && t.FixedID < 1000)
}

// sigHasUserType reports whether at least one of the signature's
// argument types is a non-kernel type. The module-scope safety rule
// for extending CORE words: an all-kernel tuple is refused — it would
// surprise importers (`add 1 {}` suddenly working from an import) and
// breaks forward compatibility the day core claims the tuple as a
// locked signature. A user-minted type or an external-builtin domain
// type (Matrix, Date, …) anchors the signature to the module's own
// domain, which is the intended use.
func sigHasUserType(s *Signature) bool {
	for i := 0; i < s.TotalArgs(); i++ {
		if !isKernelType(sigArgType(s, i)) {
			return true
		}
	}
	return false
}

// requireUserTypedSigs enforces the module-scope core-word rule over a
// set of merge candidates: every signature must carry at least one
// non-kernel argument type. word names the error source (`def` for a
// module-body merge, `import` for a transplant).
func requireUserTypedSigs(r *Registry, name, word string, sigs []Signature) error {
	for i := range sigs {
		if sigs[i].Fallback || sigHasUserType(&sigs[i]) {
			continue
		}
		return r.AqlErrorHint("extend_user_type",
			fmt.Sprintf("%s %s: a module may extend a core word only with at least one user-defined argument type per signature — %s is all core types",
				word, name, sigTupleString(&sigs[i])),
			word,
			"an all-core tuple would change what core calls mean for every importer and breaks when a future core version claims it; anchor the signature with a named type (refine / class) from your module")
	}
	return nil
}

// sigTupleString renders a signature's argument tuple as `[Integer
// String]` for the locked/conflict error messages.
func sigTupleString(s *Signature) string {
	out := "["
	for i := 0; i < s.TotalArgs(); i++ {
		if i > 0 {
			out += " "
		}
		t := sigArgType(s, i)
		if t == nil {
			t = TAny
		}
		out += t.Leaf()
	}
	return out + "]"
}

// InstallWordExtension performs the def-merge for `def name fn […]`
// where name resolves to a locked-bearing word: it constructs the word
// clone — the base word's full dispatch list merged with ext's
// overloads, compiled through the same pipeline InstallFnDef uses (body
// runner, ReturnsFn, barrier resolution), so an added signature
// dispatches exactly like an installed fn's — and binds it through the
// ordinary DefTable shadow stack. Scope (fn body / module body / top
// level), `undef`, and closure capture all fall out of the def
// machinery. Sealed words refuse.
func InstallWordExtension(r *Registry, name string, ext FnDefInfo) error {
	if IsSealedWord(name) {
		return r.AqlError("reserved_word",
			fmt.Sprintf("def %s: '%s' is a sealed word — the engine relies on its identity and it cannot be extended", name, name), "def")
	}
	base := r.Lookup(name)
	if base == nil {
		return r.AqlError("def_error",
			fmt.Sprintf("def %s: no existing word to extend", name), "def")
	}
	compiled := compileFnSigs(r, name, ext, false)
	// Module-scope safety rule: extending a CORE word from a module
	// body requires a user-defined argument type in every added
	// signature. Enforced at def time so the module author sees the
	// refusal immediately, not the importer at transplant. Top-level
	// programs (no ModuleScope) extend freely — the author is standing
	// at the point of change. Module-provided words (wrapper
	// rebindings — locked but not builtin here) are exempt: they are
	// versioned with the module dependency that owns them.
	if r.ModuleScope && r.IsBuiltinWord(name) {
		if err := requireUserTypedSigs(r, name, "def", compiled); err != nil {
			return err
		}
	}
	merged, _, err := mergeExtensionSigs(r, name, base, compiled, "")
	if err != nil {
		return err
	}
	clone := FnDefInfo{
		Name:       name,
		Signatures: merged,
		Captured:   ext.Captured,
		Extends:    name,
	}
	// Registry stays nil — the clone LIVES in r (the registry the def
	// ran in), and nil means "this registry" everywhere a name lookup
	// re-resolves the value (execFnDefLiteral's foreign-registry
	// branch). Inheriting base.Registry would be wrong for a clone of
	// a module-wrapper rebinding: dot-dispatch of the exported clone
	// would then re-look the name up in the WRAPPER's sub-registry,
	// where only the un-merged inner native exists, silently dropping
	// the merge. Per-signature execution scope is already carried by
	// each sig's compiled handler closure. resolveModuleExport tags
	// the module registry at export exactly because this is nil.
	SortSignatures(clone.Signatures)
	clone.MaxForwardArgs = calcMaxForwardArgs(clone.Signatures)
	r.Check.RecordFnBinder(name)
	r.Defs.Push(name, NewFnDef(clone))
	// Construction-time body check on the ADDED overloads only — the
	// base's signatures were checked at their own construction, and
	// re-analysing them here would duplicate their diagnostics.
	added := ext
	added.Name = name
	added.Signatures = compiled
	checkFnBodyAtConstruction(r, name, added)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
	return nil
}

// TransplantExtension installs an exported word-extension clone's
// unlocked signatures into the importing registry r as an implicit
// top-level def of the base word (§2.4). The incoming handlers stay
// closed over the module's sub-registry, so module-private helpers
// keep resolving (the module-closure rule). One level only: the
// transplant merges into r's CURRENT view of the base word and stops —
// re-export is what propagates further (the exporting module's clone
// then carries the sigs, and origin becomes that module: it takes
// ownership). Idempotent per origin; returns without installing when
// nothing new arrives.
func TransplantExtension(r *Registry, ext FnDefInfo, origin string) error {
	name := ext.Extends
	base := r.Lookup(name)
	if base == nil {
		// §4.9: the base name doesn't resolve in the importer —
		// degrade to the plain namespaced binding (no transplant).
		return nil
	}
	incoming := make([]Signature, 0, len(ext.Signatures))
	for i := range ext.Signatures {
		s := ext.Signatures[i]
		if s.Locked || s.Fallback {
			continue
		}
		incoming = append(incoming, s)
	}
	if len(incoming) == 0 {
		return nil
	}
	// Defence in depth for the module-scope core-word rule: the merge
	// that built the exported clone already refused all-kernel tuples,
	// but a transplant onto a CORE word re-checks so a hand-built /
	// host-constructed clone can't smuggle one past the importer.
	if r.IsBuiltinWord(name) {
		if err := requireUserTypedSigs(r, name, "import", incoming); err != nil {
			return err
		}
	}
	merged, changed, err := mergeExtensionSigs(r, name, base, incoming, origin)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	clone := FnDefInfo{
		Name:       name,
		Signatures: merged,
		// Registry nil for the same reason as InstallWordExtension's
		// clone: the binding lives in r, and each incoming sig's
		// handler already closes over its module sub-registry.
		Extends: name,
	}
	SortSignatures(clone.Signatures)
	clone.MaxForwardArgs = calcMaxForwardArgs(clone.Signatures)
	r.Defs.Push(name, NewFnDef(clone))
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
	return nil
}
