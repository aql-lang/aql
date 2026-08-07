// BoruError carries a stable code plus a human-readable detail. Mirrors
// borueng/go/boru_error.go in shape; we omit the source-position
// rendering and the rich expected/got formatting because the spec
// runner only matches on the code/detail substring.

export class BoruError extends Error {
  readonly code: string
  readonly detail: string
  readonly word: string

  constructor(code: string, detail: string, word = '') {
    super(`[boru/${code}]: ${detail}`)
    this.name = 'BoruError'
    this.code = code
    this.detail = detail
    this.word = word
  }
}
