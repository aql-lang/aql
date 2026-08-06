package core

// isGetWord / isGetrWord classify the two accessor word-pairs that share
// identical COMPILER and dispatch-folding behaviour but differ only in how a
// bare-word key is captured at the surface:
//
//   - `get`  / `getr`  evaluate the key (no implicit quote).
//   - `dot`  / `dotr`  quote a bare-word key as a literal field name; these
//     are what the `.` / `!.` dot-sugar lowers to (lowerReach).
//
// Every compiler/checker/VM site that special-cases the accessor family (static
// index fold, module-const fold, poly recording, None propagation, the
// list/map verb behaviours) must treat dot like get and dotr like getr — the
// folding is identical; only the surface key-capture differs. Route those
// by-name checks through these helpers so the dot pair stays a first-class
// member of the family.
func IsGetWord(name string) bool  { return name == "get" || name == "dot" }
func IsGetrWord(name string) bool { return name == "getr" || name == "dotr" }
