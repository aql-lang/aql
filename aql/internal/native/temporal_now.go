package native
import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"time"
)
// RegisterNow registers the "now" word as a standard (non-module) native word.
// now: [] -> [Instant] — returns the current UTC instant.
func RegisterNow(r *engine.Registry) {
	r.RegisterStackOnly("now", engine.Signature{
		Args: []engine.Type{},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInstant(time.Now())}, nil
		},
		Returns: []engine.Type{engine.TInstant},
	})
}
