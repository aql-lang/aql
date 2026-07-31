/-
  Demo.lean — runnable entry point for the BoruCore model.

  `BoruCore.lean` is a library (no top-level `main`, so it imports cleanly
  into the harness). This thin wrapper exposes `BoruCore.main` as the
  program entry point:

      lean --run Demo.lean       # (with BoruCore.olean on LEAN_PATH)

  or, without pre-compiling, just `#eval BoruCore.main`.
-/
import BoruCore

def main : IO Unit := BoruCore.main
