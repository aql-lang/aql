package native

import voxgigstruct "github.com/voxgig/struct"

// w9ItemsFn seams voxgigstruct.Items for items.go's non-string-key
// fallback (itemsHandler stringifies a non-string key). voxgigstruct.Items
// keys every pair with a string (map keys verbatim, list indices via
// strconv.Itoa), so the fallback is unreachable in production — the seam
// lets the *_seam9 test drive it. Defaults to the real Items.
var w9ItemsFn = voxgigstruct.Items

// W9-cluster seam (design/TEST-SEAMS.10.md). walk.go (StructUtil.walk)
// converts each traversed leaf / callback result back to a Value via
// anyToValue. Its inputs come from valueToAny, whose output anyToValue
// always accepts — so the convert-back failure arms are unreachable in
// production. w9WalkAnyToValue seams that call so the *_seam9 tests can
// force the failure. Production behaviour is unchanged: it defaults to
// the real anyToValue and production code never checks "is the seam set".
var w9WalkAnyToValue = anyToValue
