#!/usr/bin/env python3
"""
tracer.py — a "tracer bullet" differential harness for the BORU Lean model.

It drives one thin end-to-end slice through the *whole* pipeline:

    BORU source  ──tokenize──▶  Lean `Tape`  ──lean──▶  model output
        │                                                   │
        └───────────────── boru do ──────▶ engine output ───┴──▶ compare

For each program (restricted to the fragment the model covers) it runs
both the reference `boru` engine and the compiled Lean model, normalizes
their outputs, and asserts they agree. This does not *prove* the engine
matches the model — it is differential testing — but it turns the
model↔engine correspondence from "6 hand examples" into a re-runnable
check, and any divergence is a failing row pointing at exactly where the
model and the implementation disagree.

Usage:
    python3 tracer.py            # build deps, run, print a table
Env overrides:
    LEAN=/path/to/lean           # default: search PATH, then the repo's
                                 #          linked toolchain
    BORU=/path/to/boru             # default: build cmd/go/boru into a tempdir
"""
import os, re, shutil, subprocess, sys, tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent          # formal/lean/harness
LEAN_DIR = HERE.parent                            # formal/lean
REPO = LEAN_DIR.parent.parent                     # repo root

# --- the shared program set (model-covered, homogeneous-type fragment) -------
# Each entry is BORU source. Coercion cases (`add true 2`, `not 5`) are
# intentionally excluded: the model is strict there while the engine
# coerces, so they are a known, documented modeling gap — not a fragment
# the tracer bullet claims to cover.
PROGRAMS = [
    # spelling equivalence: these three must all agree (= 7)
    "sub 3 10", "10 sub 3", "10 3 sub",
    # arithmetic
    "add 1 2", "mul 4 5", "sub 10 3",
    # comparisons -> booleans
    "gt 5 3", "5 gt 3", "lt 3 5", "lt 5 3", "eq 2 2", "eq 2 3",
    # unary word, forward and from the stack
    "not true", "true not", "not false",
    # variable arity / leftover-on-stack
    "add 1 2 7", "gt 1 2 9", "1 2", "1 2 7",
    # the `end` barrier under-supplies the word
    "gt 3 end 5", "10 sub end 3",
    # underflow
    "sub",
]

WORDS = {  # source word -> Lean Word constructor
    "add": "Word.add", "sub": "Word.sub", "mul": "Word.mul",
    "gt": "Word.gtw", "lt": "Word.ltw", "eq": "Word.eqw", "not": "Word.notw",
}


def tokenize_to_tape(src: str):
    """Translate a source program into a Lean `Tape` literal, or None if it
    uses a token outside the modeled fragment."""
    items = []
    for tok in src.split():
        if re.fullmatch(r"-?\d+", tok):
            items.append(f"ilit ({tok})")
        elif tok in ("true", "false"):
            items.append(f"blit {tok}")
        elif tok in ("end", ";"):
            items.append("Item.endb")
        elif tok in WORDS:
            items.append(f"Item.word {WORDS[tok]}")
        else:
            return None
    return "[" + ", ".join(items) + "]"


def find_lean() -> str:
    if os.environ.get("LEAN"):
        return os.environ["LEAN"]
    p = shutil.which("lean")
    if p:
        return p
    # fall back to a linked toolchain laid down by the offline install
    for cand in ("/root/lean/v4.15.0/bin/lean",):
        if Path(cand).exists():
            return cand
    sys.exit("error: `lean` not found (set LEAN=...)")


def find_boru(build_dir: Path) -> str:
    if os.environ.get("BORU"):
        return os.environ["BORU"]
    out = build_dir / "boru"
    print("building boru engine ...", file=sys.stderr)
    subprocess.run(["go", "build", "-o", str(out), "./boru"],
                   cwd=REPO / "cmd" / "go", check=True)
    return str(out)


def run_model(lean: str, build_dir: Path) -> dict:
    """Compile BoruCore (re-checking the proofs) and run all programs once."""
    # 1. compile the model to an .olean so the generated file can import it.
    #    lean requires the input file to live under its cwd (module root),
    #    so compile from LEAN_DIR with a relative path; emit the olean into
    #    build_dir (which becomes the LEAN_PATH root for the import).
    subprocess.run([lean, "-o", str(build_dir / "BoruCore.olean"), "BoruCore.lean"],
                   cwd=LEAN_DIR, check=True, capture_output=True, text=True)
    # 2. generate a harness module that prints "<idx>\t<render>" per program
    lines = ["import BoruCore", "open BoruCore", ""]
    tapes = []
    for i, src in enumerate(PROGRAMS):
        tape = tokenize_to_tape(src)
        if tape is None:
            sys.exit(f"error: program {src!r} uses an unmodeled token")
        tapes.append(f'  ({i}, {tape})')
    lines.append("def progs : List (Nat × Tape) := [\n" + ",\n".join(tapes) + "\n]")
    lines.append("")
    lines.append("def main : IO Unit := do")
    lines.append("  for (i, t) in progs do")
    lines.append('    IO.println s!"{i}\\t{render (run t)}"')
    (build_dir / "Tracer.lean").write_text("\n".join(lines) + "\n")
    # 3. run it from build_dir (its module root), with LEAN_PATH pointing at
    #    the compiled olean so `import BoruCore` resolves.
    env = dict(os.environ, LEAN_PATH=str(build_dir))
    res = subprocess.run([lean, "--run", "Tracer.lean"],
                         cwd=build_dir, capture_output=True, text=True, env=env)
    if res.returncode != 0:
        sys.exit("lean harness failed:\n" + res.stderr)
    out = {}
    for line in res.stdout.splitlines():
        if "\t" in line:
            idx, val = line.split("\t", 1)
            out[int(idx)] = val
    return out


def normalize_engine(stdout: str, stderr: str, rc: int) -> str:
    text = (stdout + stderr).strip()
    if rc != 0 or text.lower().startswith("error") or "[boru/" in text:
        return "ERROR"
    return text.splitlines()[0].strip() if text else ""


def run_engine(boru: str, src: str) -> str:
    res = subprocess.run([boru, "do", src], capture_output=True, text=True)
    return normalize_engine(res.stdout, res.stderr, res.returncode)


def main() -> int:
    lean = find_lean()
    with tempfile.TemporaryDirectory() as td:
        build_dir = Path(td)
        boru = find_boru(build_dir)
        model = run_model(lean, build_dir)

        print(f"\n  {'PROGRAM':<16} {'MODEL':<10} {'ENGINE':<10} RESULT")
        print("  " + "-" * 52)
        passed = failed = 0
        for i, src in enumerate(PROGRAMS):
            m = model.get(i, "<none>")
            e = run_engine(boru, src)
            ok = (m == e)
            passed += ok
            failed += (not ok)
            print(f"  {src:<16} {m:<10} {e:<10} {'ok' if ok else 'XX MISMATCH'}")
        print("  " + "-" * 52)
        print(f"  {passed} passed, {failed} failed, {len(PROGRAMS)} total\n")
        return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
