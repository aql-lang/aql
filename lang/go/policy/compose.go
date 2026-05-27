package policy

// Compose returns a Policy that requires BOTH parent and child to
// allow a given check. This is the AWS-IAM-SCP / Linux-capability-
// bounding-set pattern: a child cannot grant more than its parent
// allows, regardless of how the child's rules are written.
//
// The composed policy is used by the aql:vm module to construct
// sub-engines: the runtime check that a sub-engine cannot exceed
// its parent's permissions is enforced structurally, by routing
// every dispatch through both layers. There is no way for a child
// rule to "lift" a parent deny — the parent always gets to vote.
//
// Both arguments must be non-nil. To express "no parent" call sites
// should pass the child unchanged (Compose is only meaningful for
// attenuation, where a real parent exists).
//
// Limits are taken from the child (more restrictive ≤ parent is the
// caller's responsibility — see RequireLimitsSubset for an optional
// pre-check). Name is reported as "<child> in <parent>".
func Compose(parent, child Policy) Policy {
	if parent == nil {
		return child
	}
	if child == nil {
		return parent
	}
	return &composed{parent: parent, child: child}
}

// composed is the runtime AND-of-both wrapper produced by Compose.
type composed struct {
	parent, child Policy
}

func (c *composed) Check(scope, op string, args Args) error {
	if err := c.parent.Check(scope, op, args); err != nil {
		return err
	}
	return c.child.Check(scope, op, args)
}

func (c *composed) CheckGlobal(name string) error {
	if err := c.parent.CheckGlobal(name); err != nil {
		return err
	}
	return c.child.CheckGlobal(name)
}

func (c *composed) CheckWord(name string) error {
	if err := c.parent.CheckWord(name); err != nil {
		return err
	}
	return c.child.CheckWord(name)
}

func (c *composed) Installed(scope string) bool {
	// AND: a scope is installed in the composed policy only if both
	// layers permit its installation.
	return c.parent.Installed(scope) && c.child.Installed(scope)
}

func (c *composed) Scope(name string) Scope {
	// Composed Scope is rarely consulted; we return the child's view
	// (with install AND'd in) so install:false on either layer is
	// visible to a Resolver consumer. Rule introspection should go
	// through the parent/child individually.
	cs := c.child.Scope(name)
	if !c.parent.Installed(name) {
		off := false
		cs.Install = &off
	}
	return cs
}

func (c *composed) Limits() Limits {
	// Take the most restrictive of parent and child for each limit.
	// Zero on either side means "no opinion"; the non-zero side wins.
	// When both set, the smaller (non-zero) value wins.
	p, ch := c.parent.Limits(), c.child.Limits()
	return Limits{
		TimeoutMs:         minNonZero(p.TimeoutMs, ch.TimeoutMs),
		MaxStepBudget:     minNonZero(p.MaxStepBudget, ch.MaxStepBudget),
		MaxStackDepth:     minNonZeroInt(p.MaxStackDepth, ch.MaxStackDepth),
		MaxMemoryBytes:    minNonZero(p.MaxMemoryBytes, ch.MaxMemoryBytes),
		MaxOutputBytes:    minNonZero(p.MaxOutputBytes, ch.MaxOutputBytes),
		MaxSubEngineDepth: minNonZeroInt(p.MaxSubEngineDepth, ch.MaxSubEngineDepth),
	}
}

func (c *composed) Name() string {
	return c.child.Name() + " in " + c.parent.Name()
}

func minNonZero(a, b int64) int64 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	}
	return b
}

func minNonZeroInt(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	}
	return b
}
