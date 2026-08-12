package eng

import core "github.com/boru-lang/boru/core/go"

// Pure test helpers copied at the carve.

func (c *entryCollector) add(e core.InterpEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func pnmSig(barrier int, args ...*core.Type) core.Signature {
	return core.Signature{Args: args, BarrierPos: barrier,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return nil, nil
		})}
}

// --- the record-time spec derivation ---------------------------------------
