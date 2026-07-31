#!/usr/bin/env python3
"""Packaging shim for the BORU Pygments lexer.

The canonical, declarative metadata lives in ``pyproject.toml``; this
``setup.py`` exists only so that legacy tooling (``python setup.py …`` and
build backends that still shell out to it) keeps working.  The important bit
is the ``pygments.lexers`` entry point ``boru = boru_lexer:BoruLexer`` — once this
package is installed, ``pygmentize -l boru`` finds the lexer automatically.
"""

from setuptools import setup

setup(
    name="boru-pygments-lexer",
    version="0.1.0",
    description=(
        "Pygments lexer for BORU, a concatenative, strongly-typed query "
        "language."
    ),
    author="BORU contributors",
    url="https://github.com/boru-lang/boru",
    license="MIT",
    py_modules=["boru_lexer"],
    python_requires=">=3.8",
    install_requires=["Pygments>=2.0"],
    entry_points={
        "pygments.lexers": ["boru = boru_lexer:BoruLexer"],
    },
    keywords=["pygments", "lexer", "boru", "syntax highlighting"],
)
