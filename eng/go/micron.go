package eng

// Scalar/Micron — the structured-scalar family.
//
// A Micron is a value that behaves as a scalar — content equality (two
// values with the same fields ARE equal), immutability, a place in the
// total order — yet is object-like, exposing named read-only
// properties via dot/get (the property signatures live in the language
// layer; the kernel owns identity, construction, and ordering).
//
// The family:
//
//	Scalar/Micron            — abstract root; `refine Micron {fields}`
//	                           constructs user kinds.
//	Scalar/Micron/Pathon     — filesystem-style path (formerly
//	                           Scalar/Path; PathonPayload).
//	Scalar/Micron/Emailon    — email address (MicronPayload:
//	                           user, host; derived address).
//	Scalar/Micron/Urlon      — absolute URL (MicronPayload: scheme,
//	                           host, port?, path?, query?, fragment?;
//	                           derived href).
//
// Every type under Micron must have a name ending in the suffix "on"
// (requireMicronName — enforced at every bind site in core_type.go /
// define_type.go). The rule is one-directional: names ending in "on"
// elsewhere in the lattice (Duration, Function, Negation, …) are not
// Microns.
//
// Construction is `make` / `refine` via the Micron Ideal
// (registerKernelIdeals): Node (map of named fields) AND parsed string
// forms are both validated, and the map form re-renders through the
// string parser so there is exactly ONE validator per kind.

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

// MicronTypeInfo is the type body produced by `refine Micron {fields}`
// — the kernel analogue of ClassTypeInfo. InstallType's Micron branch
// fills in Name/Type when the body is bound to a capitalised name.
type MicronTypeInfo struct {
	Name   string
	Type   *Type
	Fields *OrderedMap
}

// MicronPayload is the instance payload for Emailon / Urlon / user
// Microns. Fields holds only the PRIMARY fields — derived properties
// (Emailon's address, Urlon's href) are synthesized at read time and
// never stored, so content equality compares primary fields only.
// Pathon keeps its own PathonPayload (io words and the fileinfo path
// consume it).
type MicronPayload struct{ Fields *OrderedMap }

// IsMicronType reports whether v is a Micron type body (the
// `refine Micron {fields}` construction result).
func IsMicronType(v Value) bool {
	_, ok := v.Data.(MicronTypeInfo)
	return ok
}

// AsMicronType returns the MicronTypeInfo payload.
func AsMicronType(v Value) (MicronTypeInfo, error) {
	if info, ok := v.Data.(MicronTypeInfo); ok {
		return info, nil
	}
	return MicronTypeInfo{}, fmt.Errorf("AsMicronType: not a Micron type body (got %T)", v.Data)
}

// IsMicronValue reports whether v carries a MicronPayload (an Emailon /
// Urlon / user-Micron instance). Pathon values carry PathonPayload —
// probe with IsPathon.
func IsMicronValue(v Value) bool {
	_, ok := v.Data.(MicronPayload)
	return ok
}

// AsMicronFields returns the primary-field map of a MicronPayload
// instance.
func AsMicronFields(v Value) (*OrderedMap, error) {
	if p, ok := v.Data.(MicronPayload); ok {
		return p.Fields, nil
	}
	return nil, fmt.Errorf("AsMicronFields: not a Micron instance (got %T)", v.Data)
}

// requireMicronName enforces the Micron naming rule: the last
// slash-part of a name bound under Scalar/Micron must end in the
// suffix "on" (case-sensitive — `BARON` is rejected) and be at least
// three characters (rejects the bare `On`). Callers gate on the
// about-to-mint parent conforming to TMicron; the rule is
// one-directional (Micron ⇒ -on, never -on ⇒ Micron).
func requireMicronName(name string) error {
	part := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		part = name[i+1:]
	}
	if len(part) >= 3 && strings.HasSuffix(part, "on") {
		return nil
	}
	return &AqlError{
		Code:   "micron_name",
		Detail: "type " + name + ": names under Scalar/Micron must end in the suffix 'on'",
		Hint:   "rename it — e.g. " + part + "on",
	}
}

// ---- behavior ----

// micronBehavior is the one TypeBehavior for the whole family,
// installed on the root, every builtin leaf, and every user mint (leaf
// installation is load-bearing: ValuesEqual routes to Behavior only
// when both sides share a Parent with a non-default Behavior, and
// scalarSemanticEqual needs the LCA Comparer for newtype-vs-base).
//
//   - Match: nominal (embedded default — ConformsTo).
//   - Equal: content — Pathon by segments+abs, MicronPayload by field
//     map.
//   - Compare: family order. A leaf instance resolves against its own
//     kind (Pathon keeps the verbatim comparePathons ordering; other
//     kinds order by canonical render); the root opts out for
//     cross-kind pairs (ErrNoComparer → Rank fallback → cmp/lt raise
//     incomparable, tcmp orders by Rank).
//   - Format: canonical render (Emailon → address, Urlon → href, user
//     kinds → field-map render); Pathon delegates to the kernel
//     default (segments join).
type micronBehavior struct {
	defaultBehavior
	kind *Type
	// info is the field schema for user-minted kinds (set by
	// InstallType's Micron branch); nil for the root and the builtin
	// leaves. micronInstantiate walks the parent chain for it when
	// constructing a newtype of a user kind.
	info *MicronTypeInfo
}

func (micronBehavior) Format(v Value) string {
	if IsMicronValue(v) {
		return micronRender(v)
	}
	if info, ok := v.Data.(MicronTypeInfo); ok {
		// A type body renders as its bound name; an anonymous body
		// (not yet def-bound) as its schema.
		if info.Name != "" {
			return info.Name
		}
		return "Micron " + NewMap(info.Fields).String()
	}
	return kernelFormatDefault(v)
}

func (micronBehavior) Equal(a, b Value) bool {
	if ap, ok := a.Data.(PathonPayload); ok {
		if bp, ok := b.Data.(PathonPayload); ok {
			return pathonContentEqual(ap.Info, bp.Info)
		}
		return false
	}
	if am, ok := a.Data.(MicronPayload); ok {
		if bm, ok := b.Data.(MicronPayload); ok {
			return mapsEqual(am.Fields, bm.Fields)
		}
		return false
	}
	return valuesEqualDefault(a, b)
}

func (mb micronBehavior) Compare(a, b Value) (int, error) {
	// Type-literal-first rule, inside the family only (the cross-family
	// scalar catch-all on TScalar stays Rank-only).
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if IsBareTypeNode(a) && IsBareTypeNode(b) {
		return litVsLitOrder(a, b), nil
	}
	// Two concrete Microns. This Comparer owns the pair only when it
	// sits on a leaf kind — the LCA walk lands there exactly when both
	// operands share that kind (including newtype-vs-base). On the
	// root the operands are of DIFFERENT kinds: opt out, so the
	// restricted words (cmp/lt/…) raise incomparable and tcmp falls to
	// the Rank order.
	if mb.kind == nil || mb.kind.Equal(TMicron) {
		return 0, ErrNoComparer
	}
	if mb.kind.ConformsTo(TPathon) {
		return comparePathons(a, b), nil
	}
	return strings.Compare(micronCompareKey(a), micronCompareKey(b)), nil
}

// micronCompareKey is the ordering key for non-Pathon Micron leaves:
// the canonical render, computed directly (not via the receiver's
// Behavior walk, which a newtype's wrapper Behavior would reroute) so
// newtype-vs-base pairs with equal content compare equal.
func micronCompareKey(v Value) string {
	if IsMicronValue(v) {
		return micronRender(v)
	}
	return CanonValue(v)
}

// pathonContentEqual is Pathon's content equality: same segments, same
// absolute/relative flag — exactly the pairs comparePathons orders as
// 0, so eq/deq agree with cmp on paths.
func pathonContentEqual(a, b PathonInfo) bool {
	if a.Abs != b.Abs || a.Volume != b.Volume || len(a.Parts) != len(b.Parts) {
		return false
	}
	for i := range a.Parts {
		if a.Parts[i] != b.Parts[i] {
			return false
		}
	}
	return true
}

// micronRender is the canonical render for MicronPayload instances:
// Emailon → the address (user@host), Urlon → the href, user kinds →
// the field-map render. Also the Compare key for non-Pathon leaves.
func micronRender(v Value) string {
	fields, err := AsMicronFields(v)
	if err != nil {
		return kernelFormatDefault(v)
	}
	switch {
	case v.Parent.ConformsTo(TEmailon):
		return micronEmailAddress(fields)
	case v.Parent.ConformsTo(TUrlon):
		return micronURLHref(fields)
	default:
		return NewMap(fields).String()
	}
}

func micronFieldString(fields *OrderedMap, key string) string {
	v, ok := fields.Get(key)
	if !ok {
		return ""
	}
	s, _ := AsString(v)
	return s
}

// micronEmailAddress synthesizes Emailon's derived address property:
// user@host.
func micronEmailAddress(fields *OrderedMap) string {
	return micronFieldString(fields, "user") + "@" + micronFieldString(fields, "host")
}

// micronURLHref synthesizes Urlon's derived href property:
// scheme://host[:port][path][?query][#fragment].
func micronURLHref(fields *OrderedMap) string {
	var sb strings.Builder
	sb.WriteString(micronFieldString(fields, "scheme"))
	sb.WriteString("://")
	sb.WriteString(micronFieldString(fields, "host"))
	if p, ok := fields.Get("port"); ok {
		n, _ := AsInteger(p)
		sb.WriteString(":")
		sb.WriteString(strconv.FormatInt(n, 10))
	}
	sb.WriteString(micronFieldString(fields, "path"))
	if q := micronFieldString(fields, "query"); q != "" {
		sb.WriteString("?")
		sb.WriteString(q)
	}
	if f := micronFieldString(fields, "fragment"); f != "" {
		sb.WriteString("#")
		sb.WriteString(f)
	}
	return sb.String()
}

func init() {
	// Attach the family Behavior to the root and every builtin leaf.
	// Package vars (the T* nodes) initialise before init() runs, so
	// the pointers are live — same pattern as the scalar Comparer
	// installs in compare_scalar_behaviors.go.
	TMicron.Behavior = micronBehavior{kind: TMicron}
	TPathon.Behavior = micronBehavior{kind: TPathon}
	TEmailon.Behavior = micronBehavior{kind: TEmailon}
	TUrlon.Behavior = micronBehavior{kind: TUrlon}
}

// MicronProperty reads a named property of a Micron instance: the
// primary fields, plus the synthesized derived properties — Pathon's
// parts/abs, Emailon's address, Urlon's href. Derived values are
// computed at read time and never stored, so equality stays a
// primary-field comparison. Returns ok=false on a miss (the language
// layer maps a miss to `none` for get/dot and to a strict error for
// getr/dotr).
func MicronProperty(v Value, key string) (Value, bool) {
	if p, ok := v.Data.(PathonPayload); ok {
		switch key {
		case "parts":
			elems := make([]Value, len(p.Info.Parts))
			for i, s := range p.Info.Parts {
				elems[i] = NewString(s)
			}
			return NewList(elems), true
		case "abs":
			return NewBoolean(p.Info.Abs), true
		case "volume":
			// The Windows drive ("C:") or UNC root ("\\server\share");
			// empty for a POSIX / driveless path.
			return NewString(p.Info.Volume), true
		}
		return Value{}, false
	}
	fields, err := AsMicronFields(v)
	if err != nil {
		return Value{}, false
	}
	if val, ok := fields.Get(key); ok {
		return val, true
	}
	switch {
	case v.Parent.ConformsTo(TEmailon) && key == "address":
		return NewString(micronEmailAddress(fields)), true
	case v.Parent.ConformsTo(TUrlon) && key == "href":
		return NewString(micronURLHref(fields)), true
	}
	return Value{}, false
}

// MicronSchemaFor returns the field schema of the nearest user-defined
// Micron kind on t's parent chain (the schema a `refine Micron
// {fields}` construction attached at bind time). ok=false for the
// builtin leaves and the root.
func MicronSchemaFor(t *Type) (*OrderedMap, bool) {
	for ; t != nil && t.ConformsTo(TMicron); t = t.Parent {
		if mb, ok := t.Behavior.(micronBehavior); ok && mb.info != nil {
			return mb.info.Fields, true
		}
	}
	return nil, false
}

// ---- construction ----

// makeEmailon builds an Emailon from a plain-address string
// ("alice@example.com") or a {user host} map. The map form re-renders
// and re-validates through the string path — one validator.
func makeEmailon(src Value) ([]Value, error) {
	switch {
	case src.Parent.ConformsTo(TString) && IsConcrete(src):
		s, _ := AsString(src)
		return emailonFromString(s)
	case src.Parent.ConformsTo(TMap) && IsConcrete(src):
		m, err := RequireConcreteMap(src, "make Emailon")
		if err != nil {
			return nil, err
		}
		var user, host string
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			sv, serr := v.AsConcreteString()
			if serr != nil {
				return nil, &AqlError{Code: "type_error",
					Detail: fmt.Sprintf("make: Emailon field %s must be a String, got %s", k, v.String())}
			}
			switch k {
			case "user":
				user = sv
			case "host":
				host = sv
			default:
				return nil, &AqlError{Code: "type_error",
					Detail: "make: Emailon has no field " + k + " (fields: user, host)"}
			}
		}
		if user == "" || host == "" {
			return nil, &AqlError{Code: "type_error",
				Detail: "make: Emailon requires both user and host fields"}
		}
		return emailonFromString(user + "@" + host)
	}
	return nil, &AqlError{Code: "type_error",
		Detail: fmt.Sprintf("make: Emailon source must be a string or map, got %s", src.String())}
}

func emailonFromString(s string) ([]Value, error) {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return nil, &AqlError{Code: "type_error",
			Detail: fmt.Sprintf("make: invalid email address %q: %v", s, err)}
	}
	// Reject display-name / angle-bracket forms ("Alice <a@x.com>") —
	// an Emailon is the plain address only.
	if addr.Name != "" || addr.Address != s {
		return nil, &AqlError{Code: "type_error",
			Detail: fmt.Sprintf("make: Emailon takes a plain user@host address, got %q", s)}
	}
	at := strings.LastIndex(s, "@")
	fields := NewOrderedMap()
	fields.Set("user", NewString(s[:at]))
	fields.Set("host", NewString(s[at+1:]))
	return []Value{NewValueRaw(TEmailon, MicronPayload{Fields: fields})}, nil
}

// urlonFieldOrder is the canonical field layout of an Urlon map source
// (and of the stored payload — optional fields are simply absent).
var urlonFieldOrder = []string{"scheme", "host", "port", "path", "query", "fragment"}

// makeUrlon builds an Urlon from an absolute-URL string or a
// {scheme host port? path? query? fragment?} map. The map form
// re-renders to an href and re-parses through the string path — one
// validator.
func makeUrlon(src Value) ([]Value, error) {
	switch {
	case src.Parent.ConformsTo(TString) && IsConcrete(src):
		s, _ := AsString(src)
		return urlonFromString(s)
	case src.Parent.ConformsTo(TMap) && IsConcrete(src):
		m, err := RequireConcreteMap(src, "make Urlon")
		if err != nil {
			return nil, err
		}
		parts := map[string]string{}
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			valid := false
			for _, f := range urlonFieldOrder {
				if k == f {
					valid = true
					break
				}
			}
			if !valid {
				return nil, &AqlError{Code: "type_error",
					Detail: "make: Urlon has no field " + k + " (fields: " + strings.Join(urlonFieldOrder, ", ") + ")"}
			}
			if k == "port" {
				n, nerr := v.AsConcreteInteger()
				if nerr != nil {
					return nil, &AqlError{Code: "type_error",
						Detail: fmt.Sprintf("make: Urlon port must be an Integer, got %s", v.String())}
				}
				parts[k] = strconv.FormatInt(n, 10)
				continue
			}
			sv, serr := v.AsConcreteString()
			if serr != nil {
				return nil, &AqlError{Code: "type_error",
					Detail: fmt.Sprintf("make: Urlon field %s must be a String, got %s", k, v.String())}
			}
			parts[k] = sv
		}
		if parts["scheme"] == "" || parts["host"] == "" {
			return nil, &AqlError{Code: "type_error",
				Detail: "make: Urlon requires both scheme and host fields"}
		}
		href := parts["scheme"] + "://" + parts["host"]
		if parts["port"] != "" {
			href += ":" + parts["port"]
		}
		href += parts["path"]
		if parts["query"] != "" {
			href += "?" + parts["query"]
		}
		if parts["fragment"] != "" {
			href += "#" + parts["fragment"]
		}
		return urlonFromString(href)
	}
	return nil, &AqlError{Code: "type_error",
		Detail: fmt.Sprintf("make: Urlon source must be a string or map, got %s", src.String())}
}

func urlonFromString(s string) ([]Value, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, &AqlError{Code: "type_error",
			Detail: fmt.Sprintf("make: invalid URL %q: %v", s, err)}
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, &AqlError{Code: "type_error",
			Detail: fmt.Sprintf("make: Urlon requires an absolute URL (scheme://host…), got %q", s)}
	}
	fields := NewOrderedMap()
	fields.Set("scheme", NewString(u.Scheme))
	fields.Set("host", NewString(u.Hostname()))
	if p := u.Port(); p != "" {
		n, perr := strconv.ParseInt(p, 10, 64)
		if perr != nil {
			return nil, &AqlError{Code: "type_error",
				Detail: fmt.Sprintf("make: invalid URL port %q", p)}
		}
		fields.Set("port", NewInteger(n))
	}
	if u.Path != "" {
		fields.Set("path", NewString(u.Path))
	}
	if u.RawQuery != "" {
		fields.Set("query", NewString(u.RawQuery))
	}
	if u.Fragment != "" {
		fields.Set("fragment", NewString(u.Fragment))
	}
	return []Value{NewValueRaw(TUrlon, MicronPayload{Fields: fields})}, nil
}

// makeMicronUser instantiates a user-defined Micron kind from a
// concrete map of fields. Unknown and missing fields error; each field
// validates against its schema constraint via MakeClassFieldValue (the
// same per-field rules class construction applies — concrete defaults,
// nominal class fields, predicate types).
func makeMicronUser(info MicronTypeInfo, data Value, r *Registry) ([]Value, error) {
	kindName := info.Name
	if kindName == "" {
		kindName = "Micron"
	}
	if !data.Parent.ConformsTo(TMap) || !IsConcrete(data) {
		return nil, &AqlError{Code: "type_error",
			Detail: fmt.Sprintf("make: %s takes a map of fields, got %s", kindName, data.String())}
	}
	provided, err := RequireConcreteMap(data, "make "+kindName)
	if err != nil {
		return nil, err
	}
	for _, key := range provided.Keys() {
		if _, ok := info.Fields.Get(key); !ok {
			return nil, &AqlError{Code: "type_error",
				Detail: fmt.Sprintf("make: %s has no field %q", kindName, key)}
		}
	}
	result := NewOrderedMap()
	for _, key := range info.Fields.Keys() {
		constraint, _ := info.Fields.Get(key)
		val, ok := provided.Get(key)
		if !ok {
			// A concrete (non-type-body) schema entry is a default.
			if constraint.Data != nil && !IsTypeBody(constraint) {
				result.Set(key, constraint)
				continue
			}
			return nil, &AqlError{Code: "type_error",
				Detail: fmt.Sprintf("make: %s is missing field %q", kindName, key)}
		}
		checked, cerr := MakeClassFieldValue(val, constraint, r)
		if cerr != nil {
			return nil, &AqlError{Code: "type_error",
				Detail: fmt.Sprintf("make: %s field %q: %v", kindName, key, cerr)}
		}
		result.Set(key, checked)
	}
	parent := info.Type
	if parent == nil {
		parent = TMicron
	}
	return []Value{NewValueRaw(parent, MicronPayload{Fields: result})}, nil
}

// CheckMicronConstruction is the CHECK-MODE mirror of the Micron
// constructors (the analogue of CheckMakeConstruction for class /
// Resource targets): when the make target is a Micron kind and the
// source is fully concrete, the whole validation — string parsing,
// unknown/missing fields, per-field types, the abstract root — is
// decidable right here, so the real constructor runs and its error
// surfaces as a type_error diagnostic with the byte-identical runtime
// message. A carrier source, or a map source carrying any carrier
// field, is value-dependent and stays with the runtime constructor.
// Deduped by detail+position. No-op outside check mode.
func CheckMicronConstruction(r *Registry, target, src Value, pos SrcPos) {
	if r == nil || !r.Check.IsActive() {
		return
	}
	target = ResolveTypeLiteralDef(target, r)
	if !micronAccepts(target) {
		return
	}
	if !IsConcrete(src) {
		return
	}
	if src.Parent.ConformsTo(TMap) {
		m, _ := AsMap(src)
		if m == nil {
			return
		}
		for _, k := range m.Keys() {
			if fv, ok := m.Get(k); ok && !IsConcrete(fv) {
				return
			}
		}
	}
	// The Micron constructors are pure (no registry mutation, no I/O),
	// so running the real one at analysis time is safe.
	if _, err := micronInstantiate(target, src, r); err != nil {
		code, detail := "type_error", err.Error()
		var ae *AqlError
		if errors.As(err, &ae) {
			code, detail = ae.Code, ae.Detail
		}
		CheckAddUniqueDiagnostic(r, code, detail, "make", pos)
	}
}

// CheckAddUniqueDiagnostic adds a check-mode diagnostic unless an
// identical one (code+detail+position) is already recorded — ReturnsFns
// run once per analysed call shape, and a body can be analysed under
// several shapes. Every caller mirrors a GUARANTEED runtime error over
// exactly-known operands, so the diagnostic is stamped RuntimeMirror
// (the compile pipeline does not refuse on it — the recording model is
// exact) and inside an error-catching `do` body AddDiagnostic
// re-attributes it to a caught info finding. A caught (downgraded)
// entry never blocks a later REAL emission of the same finding at
// another site, so the dedupe skips it.
func CheckAddUniqueDiagnostic(r *Registry, code, detail, word string, pos SrcPos) {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code && d.Detail == detail && d.Row == pos.Row && d.Col == pos.Col &&
			!d.CaughtAtRuntime {
			return
		}
	}
	r.Check.AddDiagnostic(CheckDiagnostic{
		Code:          code,
		Detail:        detail,
		Word:          word,
		Row:           pos.Row,
		Col:           pos.Col,
		RuntimeMirror: true,
	})
}

// ---- the Micron Ideal (refine / make dispatch) ----

// micronAccepts is the Ideal dispatch predicate: a bare lattice node
// under Scalar/Micron, or a MicronTypeInfo type body. Deliberately
// narrow — kernel Ideals win Accepts ties, so this must not overclaim.
func micronAccepts(v Value) bool {
	if IsMicronType(v) {
		return true
	}
	return IsBareTypeNode(v) && (&v).ConformsTo(TMicron)
}

// micronConstruct is `refine ‹base› {fields}` for the family.
// Schema-carrying refinement is allowed of the ROOT only — a leaf's
// field set is its validation contract (D5). Bare nominal refinement
// of any Micron kind rides the ordinary refine-prefab path instead.
func micronConstruct(base, arg Value, r *Registry) ([]Value, error) {
	if !(IsBareTypeNode(base) && base.Equal(TMicron)) {
		name := base.Parent.Name
		if IsBareTypeNode(base) {
			name = base.Name
		} else if info, err := AsMicronType(base); err == nil && info.Name != "" {
			name = info.Name
		}
		return nil, &AqlError{Code: "type_error",
			Detail: "refine: " + name + " is a Micron kind — its field set is its validation contract",
			Hint:   "newtype it (def Newon refine " + name + ") or define a fresh kind: def Nameon refine Micron {field:Type}"}
	}
	fieldsIn, err := RequireConcreteMap(arg, "refine Micron")
	if err != nil {
		return nil, &AqlError{Code: "type_error",
			Detail: "refine Micron: takes a map of fields, e.g. refine Micron {foo:String}"}
	}
	fields := NewOrderedMap()
	for _, k := range fieldsIn.Keys() {
		v, _ := fieldsIn.Get(k)
		v = ResolveWordValue(v)
		if !IsTypeBody(v) && !IsConcrete(v) {
			return nil, &AqlError{Code: "type_error",
				Detail: fmt.Sprintf("refine Micron: field %s must be a type or a concrete default, got %s", k, v.String())}
		}
		fields.Set(k, v)
	}
	return []Value{NewValueRaw(TMicron, MicronTypeInfo{Fields: fields})}, nil
}

// micronInstantiate is `make ‹kind› data` for the family.
func micronInstantiate(typ, data Value, r *Registry) ([]Value, error) {
	if IsMicronType(typ) {
		info, _ := AsMicronType(typ)
		return makeMicronUser(info, data, r)
	}
	kind := CanonicalType(r, &typ)
	switch {
	case kind.Equal(TMicron):
		return nil, &AqlError{Code: "type_error",
			Detail: "make: Micron is abstract — construct a leaf (Pathon / Emailon / Urlon) or a user-defined Micron kind",
			Hint:   "define one with: def Nameon refine Micron {field:Type}"}
	case kind.Equal(TPathon):
		return makePathon(data, false)
	case kind.Equal(TEmailon):
		return makeEmailon(data)
	case kind.Equal(TUrlon):
		return makeUrlon(data)
	}
	// A newtype (bare nominal refine) of a builtin leaf or of a user
	// kind: construct the base, then tag the result with the newtype —
	// the same reparent the typed-def path (`def x:Foo v`) performs.
	for t := kind; t != nil && t.ConformsTo(TMicron); t = t.Parent {
		var out []Value
		var err error
		switch {
		case t.Equal(TPathon):
			out, err = makePathon(data, false)
		case t.Equal(TEmailon):
			out, err = makeEmailon(data)
		case t.Equal(TUrlon):
			out, err = makeUrlon(data)
		default:
			if mb, ok := t.Behavior.(micronBehavior); ok && mb.info != nil {
				out, err = makeMicronUser(*mb.info, data, r)
			} else {
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		return []Value{ReparentValue(out[0], kind)}, nil
	}
	return nil, &AqlError{Code: "type_error",
		Detail: "make: " + kind.Name + " has no field schema — define one with refine Micron {fields}"}
}
