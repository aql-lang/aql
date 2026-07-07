// Test seam (design/TEST-SEAMS.10.md): jsonMarshalIndent seams
// json.MarshalIndent for Do's encode-failure arm. The parsed jsonic
// map is always plain JSON-shaped data, so it always marshals
// cleanly through the public surface — only a seam can observe the
// error arm.

package prep

import "encoding/json"

var jsonMarshalIndent = json.MarshalIndent
