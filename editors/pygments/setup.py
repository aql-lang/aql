#!/usr/bin/env python3
"""Packaging shim for the AQL Pygments lexer.

The canonical, declarative metadata lives in ``pyproject.toml``; this
``setup.py`` exists only so that legacy tooling (``python setup.py …`` and
build backends that still shell out to it) keeps working.  The important bit
is the ``pygments.lexers`` entry point ``aql = aql_lexer:AqlLexer`` — once this
package is installed, ``pygmentize -l aql`` finds the lexer automatically.
"""

from setuptools import setup

setup(
    name="aql-pygments-lexer",
    version="0.1.0",
    description=(
        "Pygments lexer for AQL, a concatenative, strongly-typed query "
        "language."
    ),
    author="AQL contributors",
    url="https://github.com/aql-lang/aql",
    license="MIT",
    py_modules=["aql_lexer"],
    python_requires=">=3.8",
    install_requires=["Pygments>=2.0"],
    entry_points={
        "pygments.lexers": ["aql = aql_lexer:AqlLexer"],
    },
    keywords=["pygments", "lexer", "aql", "syntax highlighting"],
)
