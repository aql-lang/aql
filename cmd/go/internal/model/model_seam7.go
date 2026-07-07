// Test seam (design/TEST-SEAMS.10.md): jsonMarshalIndent seams
// json.MarshalIndent for the --print path's encode-failure arm. The
// resolved model is plain JSON-shaped data (maps/slices/scalars) from
// jsonic unification, so it always marshals cleanly through the
// public CLI surface — only a seam can observe the error arm.

package model

import "encoding/json"

var jsonMarshalIndent = json.MarshalIndent
