/-
  Demo.lean — runnable entry point for the AqlCore model.

  `AqlCore.lean` is a library (no top-level `main`, so it imports cleanly
  into the harness). This thin wrapper exposes `AqlCore.main` as the
  program entry point:

      lean --run Demo.lean       # (with AqlCore.olean on LEAN_PATH)

  or, without pre-compiling, just `#eval AqlCore.main`.
-/
import AqlCore

def main : IO Unit := AqlCore.main
