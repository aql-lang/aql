package vault

// confirm.go — the destructive-action confirmation policy.
//
// Any operation that can lose secret data, lose access, or roll back
// state is confirmed deliberately:
//
//	tier 1 (standard, e.g. rm/rotate/overwrite): on a TTY, type "yes";
//	    non-interactively, pass --yes/-y.
//	tier 2 (irreversible, e.g. password rm/--rekey/restore/prune): on a
//	    TTY, type the exact target token; non-interactively, pass --yes/-y
//	    together with the operation's own deliberate flag (--rekey/--prune/
//	    --generation/--force); for a verb with no such flag (plain
//	    `password rm`), --yes plus the explicit target argument is the
//	    signal, so callers pass deliberate=true there.
//
// Passphrase env vars (BORU_VAULT_PASSPHRASE / BORU_VAULT_NEW_PASSPHRASE)
// authenticate but do NOT satisfy destructive confirmation — the
// deliberate flag is still required when there is no TTY.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/auth"
)

// errAborted is returned when the user declines a confirmation prompt.
var errAborted = errors.New("aborted")

const (
	confirmTier1 = 1
	confirmTier2 = 2
)

// stdinIsTTY reports whether r is an interactive terminal.
func stdinIsTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && isTerminal(int(f.Fd()))
}

// confirmDestructive gates a destructive operation per the policy above.
// It returns nil to proceed, errAborted if the user declines, or a
// descriptive error naming the missing flag in non-interactive use.
//
//	tier:       confirmTier1 or confirmTier2
//	target:     the exact token a tier-2 TTY prompt requires (e.g. the
//	            slot name, or a generation); ignored for tier 1
//	what:       a short noun phrase for the prompt ("delete secret %q")
//	yes:        the --yes/-y flag
//	deliberate: tier-2 only — whether the op's deliberate flag was given
func confirmDestructive(tier int, target, what string, yes, deliberate bool, stdin io.Reader, stdout io.Writer) error {
	if stdinIsTTY(stdin) {
		ir := auth.NewInputReader(stdin)
		if tier == confirmTier2 {
			ans, err := ir.ReadLine(fmt.Sprintf("About to %s. Type %q to confirm: ", what, target), stdout)
			if err != nil {
				return err
			}
			if strings.TrimSpace(ans) != target {
				return errAborted
			}
			return nil
		}
		ans, err := ir.ReadLine(fmt.Sprintf("About to %s. Type 'yes' to confirm: ", what), stdout)
		if err != nil {
			return err
		}
		if strings.TrimSpace(ans) != "yes" {
			return errAborted
		}
		return nil
	}

	// Non-interactive: require the deliberate flag(s).
	if tier == confirmTier2 {
		if yes && deliberate {
			return nil
		}
		return fmt.Errorf("refusing to %s without confirmation: pass --yes together with the operation flag (no terminal to prompt on)", what)
	}
	if yes {
		return nil
	}
	return fmt.Errorf("refusing to %s without confirmation: pass --yes (no terminal to prompt on)", what)
}
