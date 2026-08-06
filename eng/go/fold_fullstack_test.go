package eng

// fold_fullstack_test.go pins EmitState.FoldFullStack — the static fold that
// graduated the full-stack words (depth/pick/roll) for provably-exact stacks
// (checker-compiler-completeness-review.0.md §8.2(2)). White-box per the
// emit_dynapply_fnunit pattern: every gate arm and every word's fold,
// positive and negative.
