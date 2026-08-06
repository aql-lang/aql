package eng

// Pure test helpers copied at the carve.

func (c *entryCollector) add(e InterpEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func pnmSig(barrier int, args ...*Type) Signature {
	return Signature{Args: args, BarrierPos: barrier,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		})}
}

// --- the record-time spec derivation ---------------------------------------
