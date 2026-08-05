package eng

// Compiler-piece pass arming (Stage 3b of the four-piece split): the
// compile-pass ritual and the hermetic emit swap. These construct the
// compiler concrete EmitState, so they live compiler-side; at the package
// cut they become free functions over the check state
// (compiler.BeginCompilePass(c)), keeping the one-shared-helper rule
// (Vm.compile shipped without the Compiling flag from a hand-rolled copy).

// BeginCompilePass is Begin() plus the compile-pass arming ritual shared
// by every bytecode-recording entry point (lang's CompileCheck, boru:vm's
// Vm.compile): install a fresh EmitState, mark the pass as Compiling, and
// drop the fn-body memos so bodies re-analyse — and re-record — under
// THIS pass (a summary cached by an earlier plain check would leave its
// compiled unit empty). One shared helper is what keeps the ritual's
// pieces from going missing in a hand-rolled copy: Vm.compile shipped
// without the Compiling flag for exactly that reason.
func (c *CheckState) BeginCompilePass() func() {
	done := c.Begin()
	if c == nil {
		return done
	}
	c.Emit = NewEmitState()
	c.Compiling = true
	c.FnSummaries = nil
	c.FnInflight = nil
	return done
}

// IsolateEmit swaps in a FRESH EmitState (sharing the registry) for the
// duration of a throwaway evaluation, returning a restore func. It is the
// hermetic complement to Suspend: Suspend keeps the SAME EmitState (only
// stopping recording), so a nested eval's interned consts and RememberOriginal
// entries still pollute the live EmitState's pool. The dynamic-help example
// eval fires from OnRegisterHook on EVERY fn registration — including the
// program's own `def f fn […]` DURING compilation — so without a swap its
// example run leaks compile-time state (e.g. a generated `['a' 'b']` sample
// list) into the program's own EmitState, corrupting a later operand's compile.
// Swapping to a throwaway pool, discarded on restore, contains it fully.
func (c *CheckState) IsolateEmit() func() {
	if c == nil {
		return func() {}
	}
	saved := c.Emit
	c.Emit = newIsolatedEmit(c.Recorder())
	return func() { c.Emit = saved }
}
