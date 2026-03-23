
# AQL Mini-Language Syntax Design

Inline mini-language literals for regular expressions, path queries,
transliteration, and other embedded notations.


## Syntax

A mini-language literal starts with a two-letter lowercase prefix
followed by `/`, and consumes all characters up to the next
whitespace. The general form is:

```
xy/...
```

Where `x` and `y` are lowercase ASCII letters. The `/` is part of
the prefix, not a delimiter — the content after the prefix may use
`/` as an internal separator depending on the specific mini-language.

Examples:

```
rm/a+/            # regexp match
rs/foo/bar/g      # regexp substitute, global
xp//div[@id]      # xpath
jp/$.store.book   # jsonpath
jq/.[] | .name    # jq filter
tr/a-z/A-Z/       # transliterate
```

Mini-language literals are **not** strings. They parse into typed
values that carry both the raw content and the mini-language
identifier. The engine uses the prefix to determine which word
or type to produce.


## Lexer Integration

### Custom jsonic LexMatcher

Register a lex matcher that recognizes the `xy/` pattern before
jsonic's default tokenization. The matcher:

1. Checks that the current position has two lowercase letters
   followed by `/`.
2. Consumes all characters up to the next whitespace (space, tab,
   newline) or end of source.
3. Produces a custom token carrying the full literal text.

Pseudocode for the lex matcher:

```
if pos+2 < len(src)
   && isLower(src[pos]) && isLower(src[pos+1])
   && src[pos+2] == '/'
then
   end = next whitespace or EOF after pos
   emit token with src[pos:end]
```

Registration in `parse.go`:

```go
TinML := j.Token("#ML", "")  // mini-language token, no fixed text

j.Lex(func(src string, pos int, ctx *jsonic.Context) (int, jsonic.Tin) {
    if pos+2 < len(src) &&
       src[pos] >= 'a' && src[pos] <= 'z' &&
       src[pos+1] >= 'a' && src[pos+1] <= 'z' &&
       src[pos+2] == '/' {
        end := pos + 3
        for end < len(src) && !isWhitespace(src[end]) {
            end++
        }
        return end - pos, TinML
    }
    return 0, 0 // no match
})
```

Then extend the `"val"` rule to handle `TinML`:

```go
{S: [][]jsonic.Tin{{TinML}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
    raw := ctx.Src()[ctx.From():ctx.To()]
    r.Node = jsonic.Text{Str: raw, Quote: "mini"}
}}
```

The converter layer detects `Quote: "mini"` and produces a
`MiniLang` value with the prefix and content separated.


## Value Representation

```go
type MiniLang struct {
    Prefix  string   // "rm", "rs", "xp", etc.
    Content string   // everything after the prefix "xy/"
    Raw     string   // full original text
}
```

Type hierarchy:

```
Scalar/MiniLang                  # base mini-language type
Scalar/MiniLang/RegExp           # rm/, rs/
Scalar/MiniLang/XPath            # xp/
Scalar/MiniLang/JsonPath         # jp/
Scalar/MiniLang/Jq               # jq/
Scalar/MiniLang/Translate        # tr/
Scalar/MiniLang/Format           # fm/
Scalar/MiniLang/Glob             # gl/
Scalar/MiniLang/Url              # ur/
Scalar/MiniLang/Shell            # sh/
Scalar/MiniLang/Date             # dt/
Scalar/MiniLang/Css              # cs/
```


## Mini-Languages

### rm — Regexp Match

Match a regular expression against a string. Returns a match
structure or `none`.

Syntax: `rm/pattern/flags`

Flags: `i` (case insensitive), `m` (multiline), `s` (dot matches
newline).

```
rm/a+/                   # match one or more 'a'
rm/(\d+)-(\d+)/          # match with capture groups
rm/hello/i               # case insensitive
```

Usage:

```
"aardvark" rm/a+/        => RegExpMatch{match:"aa", groups:[], pos:0}
"foo" rm/a+/             => none
```

Signature: `[string, minilang/regexp] -> [any]`

The match result is a map-like structure:

```
"abc-123" rm/([a-z]+)-(\d+)/
=> {match:"abc-123", groups:["abc","123"], pos:0}
```

Access fields with `dot`:

```
"abc-123" rm/([a-z]+)-(\d+)/ dot match    => "abc-123"
"abc-123" rm/([a-z]+)-(\d+)/ dot groups   => ["abc","123"]
```

### rs — Regexp Substitute

Replace matches of a pattern in a string.

Syntax: `rs/pattern/replacement/flags`

Flags: `g` (global — replace all), `i` (case insensitive).

```
"aabaa" rs/a+/x/         => "xbaa"
"aabaa" rs/a+/x/g        => "xbx"
"Hello" rs/hello/bye/i   => "bye"
```

Backreferences use `$1`, `$2`, etc:

```
"2025-03-23" rs/(\d{4})-(\d{2})-(\d{2})/$3.$2.$1/
=> "23.03.2025"
```

Signature: `[string, minilang/regexp] -> [string]`

### rt — Regexp Test

Test whether a pattern matches. Returns boolean.

Syntax: `rt/pattern/flags`

```
"hello" rt/ell/           => true
"hello" rt/xyz/           => false
```

Signature: `[string, minilang/regexp] -> [boolean]`

### rf — Regexp Find All

Return all matches of a pattern.

Syntax: `rf/pattern/flags`

```
"a1b2c3" rf/\d+/          => ["1","2","3"]
```

Signature: `[string, minilang/regexp] -> [list]`

### tr — Transliterate

Character-by-character translation, following Perl `tr`/`y`
semantics.

Syntax: `tr/from/to/flags`

Flags: `d` (delete unmatched), `s` (squeeze duplicates),
`c` (complement).

```
"hello" tr/a-z/A-Z/       => "HELLO"
"hello" tr/aeiou/*/        => "h*ll*"
"aabbcc" tr/abc/xyz/s      => "xyz"
```

Signature: `[string, minilang/translate] -> [string]`

### xp — XPath Query

Apply an XPath expression to an XML string or parsed document.

Syntax: `xp/expression`

```
"<root><a>1</a><b>2</b></root>" xp//a/text()
=> "1"
```

Signature: `[any, minilang/xpath] -> [any]`

### jp — JsonPath Query

Apply a JsonPath expression to a map or list.

Syntax: `jp/expression`

```
{store:{book:[{title:"A"},{title:"B"}]}} jp/$.store.book[*].title
=> ["A","B"]

{a:{b:{c:1}}} jp/$.a.b.c
=> 1
```

Signature: `[any, minilang/jsonpath] -> [any]`

### jq — Jq Filter

Apply a jq expression to a map or list.

Syntax: `jq/expression`

```
[{a:1,b:2},{a:3,b:4}] jq/.[]|.a
=> [1,3]

{name:"alice",age:30} jq/{greeting:("hello,"+.name)}
=> {greeting:"hello,alice"}
```

Signature: `[any, minilang/jq] -> [any]`

### gl — Glob Pattern

File glob matching. Returns boolean when applied to a string, or
a list of matches when applied to a file path context.

Syntax: `gl/pattern`

```
"foo.txt" gl/*.txt         => true
"image.png" gl/*.txt       => false
```

Signature: `[string, minilang/glob] -> [boolean]`

### fm — Format String

Positional string formatting. Placeholders `{}` are filled from
the stack or a list.

Syntax: `fm/template`

```
"world" fm/hello,{}/       => "hello,world"
["a","b"] fm/{}-{}/        => "a-b"
```

Signature: `[any, minilang/format] -> [string]`

### ur — URL Pattern

Parse or match URL patterns. Useful for routing.

Syntax: `ur/pattern`

```
"https://example.com/user/42" ur//user/{id}
=> {id:"42"}
```

Signature: `[string, minilang/url] -> [any]`

### dt — Date/Time Format

Parse or format date/time strings.

Syntax: `dt/format`

```
"2025-03-23" dt/YYYY-MM-DD/   => DateValue
```

Signature: `[string, minilang/date] -> [any]`

### cs — CSS Selector

Apply a CSS selector to an HTML string or parsed document.

Syntax: `cs/selector`

```
"<div><p class='x'>hi</p></div>" cs/p.x
=> "hi"
```

Signature: `[any, minilang/css] -> [any]`

### sh — Shell Pattern

POSIX shell-style pattern matching (fnmatch).

Syntax: `sh/pattern`

```
"hello.txt" sh/hello.*/     => true
"readme.md" sh/*.md/        => true
```

Signature: `[string, minilang/shell] -> [boolean]`


## Prefix Registry

The set of recognized prefixes is extensible. A registry maps
two-letter prefixes to:

1. A **parser** that splits the content into structured fields
   (e.g., `rs/` splits on unescaped `/` to get pattern,
   replacement, and flags).
2. A **type** under the `MiniLang` hierarchy.
3. An optional **word** that is automatically invoked when the
   literal appears in suffix position.

```go
var miniLangRegistry = map[string]MiniLangDef{
    "rm": {Type: TRegExp,     Parser: parseRegExp},
    "rs": {Type: TRegExp,     Parser: parseRegExpSub},
    "rt": {Type: TRegExp,     Parser: parseRegExp},
    "rf": {Type: TRegExp,     Parser: parseRegExp},
    "tr": {Type: TTranslate,  Parser: parseTranslate},
    "xp": {Type: TXPath,      Parser: parseRaw},
    "jp": {Type: TJsonPath,   Parser: parseRaw},
    "jq": {Type: TJq,         Parser: parseRaw},
    "gl": {Type: TGlob,       Parser: parseRaw},
    "fm": {Type: TFormat,     Parser: parseRaw},
    "ur": {Type: TUrl,        Parser: parseRaw},
    "dt": {Type: TDate,       Parser: parseRaw},
    "cs": {Type: TCss,        Parser: parseRaw},
    "sh": {Type: TShell,      Parser: parseRaw},
}
```

Unrecognized prefixes produce a generic `MiniLang` value. Users
can define custom mini-language handlers via `fn` overloading on
the `MiniLang` type.


## Content Parsing Rules

### Delimiter-based mini-languages

`rm`, `rs`, `rt`, `rf`, `tr` use `/` as an internal delimiter:

```
rs/pattern/replacement/flags
```

Escaped slashes (`\/`) are literal. The parser splits on unescaped
`/` characters to extract fields. The first `/` (after the prefix)
begins the content; subsequent `/` separate fields.

Parsing `rs/foo\/bar/baz/gi`:
- pattern: `foo/bar` (escaped slash preserved)
- replacement: `baz`
- flags: `gi`

### Path-based mini-languages

`xp`, `jp`, `jq`, `cs` treat the entire content after `xy/` as a
single expression. Internal `/` characters are part of the
expression, not delimiters.

```
xp//div/span[@class='x']     # content: /div/span[@class='x']
jp/$.store.book[0].title      # content: $.store.book[0].title
```

### Simple mini-languages

`gl`, `fm`, `ur`, `dt`, `sh` use the raw content directly.


## Interaction with AQL

### Suffix Precedence

Mini-language literals work as suffix arguments to the value they
operate on, following standard AQL suffix collection:

```
"hello world" rm/\w+/       # rm/ is suffix arg to string
"aabaa" rs/a+/x/g           # rs/ is suffix arg to string
```

The type system dispatches: when a `string` is on the stack and a
`MiniLang/RegExp` follows in suffix position, the engine matches
the appropriate signature.

### As Standalone Values

Mini-language literals can also be pushed to the stack and used
with explicit words:

```
def pattern rm/\d+/
"abc123" pattern             # error: pattern is not a word
"abc123" do [pattern]        # evaluates pattern, then...
```

More naturally, store and retrieve:

```
set pat rm/\d+/
"abc123" get pat             # uses stored pattern
```

### Composition with Array Words

From ARRAYIFICATION.md, mini-languages compose with array operations:

```
each [rm/\d+/] ["abc1","def2","ghi"]
=> [{match:"1",...},{match:"2",...},none]

compress each [rt/\d/] ["abc","a1","xyz","b2"]
=> ["a1","b2"]

each [rs/\s+/_/g] ["hello world","foo  bar"]
=> ["hello_world","foo_bar"]
```

### In Function Definitions

```
fn is-email [
    [string] [boolean]
    [rt/^[^@]+@[^@]+\.[^@]+$/]
]

is-email "foo@bar.com"       => true
is-email "not an email"      => false
```


## Escaping and Edge Cases

### Whitespace Terminates

The literal ends at whitespace. Content cannot contain literal
spaces. For patterns needing spaces, use `\s` or `\ ` (backslash
space) where the mini-language supports it.

```
rm/hello\sworld/            # matches "hello world"
```

### Empty Content

`rm/` with no content after the prefix is valid — it produces a
MiniLang value with empty content. Whether this is meaningful
depends on the specific mini-language.

### Collision with Existing Words

Two-letter words followed by `/` could collide. The lex matcher
fires before word resolution, so `rm/` is always a mini-language
literal, never the word `rm` followed by a `/`. If a two-letter
word is needed alongside mini-languages, it must be separated from
any `/`:

```
rm /foo      # word "rm", then atom "/foo" (space separates)
rm/foo/      # mini-language literal
```

Existing AQL words are generally longer than two characters, so
collisions should be rare. Reserved prefixes should be documented.

### Nested Brackets and Quotes

Content between the prefix and whitespace is taken raw. Brackets
`[]`, braces `{}`, and quotes within the content are not parsed by
jsonic — the lex matcher consumes them as part of the literal.

```
jp/$[?(@.price<10)]         # brackets are part of the jsonpath
jq/.[]|select(.a>1)         # pipes and parens are literal
```


## Implementation Priority

### Phase 1 — Lexer and Type

- Custom lex matcher for `xy/` pattern
- `MiniLang` value type and type hierarchy
- Converter integration (Quote: "mini")
- Prefix registry infrastructure

### Phase 2 — Regexp Family

- `rm/` match, `rs/` substitute, `rt/` test, `rf/` find-all
- Content parser for `/`-delimited fields
- Go `regexp` backend
- Match result structure and field access

### Phase 3 — Path Queries

- `jp/` jsonpath on maps/lists
- `xp/` xpath on strings/documents
- `jq/` jq expressions
- `cs/` CSS selectors

### Phase 4 — Utilities

- `tr/` transliteration
- `gl/` glob matching
- `fm/` format strings
- `ur/` URL pattern matching
- `dt/` date/time formatting
- `sh/` shell pattern matching


## Summary

The `xy/` mini-language syntax gives AQL a compact, recognizable
notation for embedded domain-specific operations. The two-letter
prefix is mnemonic and terse. The whitespace-terminated lexing rule
is simple to implement and unambiguous. The type system routes
mini-language values to the correct operation through standard
signature matching, and the prefix registry makes the set of
mini-languages extensible without parser changes.
