// BoruError carries a stable code, a human-readable detail, and structured
// diagnostic metadata. Its message intentionally remains the stable first
// line consumed by existing hosts; richer renderers can read the fields.

export interface DiagSuggestion {
  message: string;
  replacement?: string;
}

export interface BoruErrorMetadata {
  row?: number;
  col?: number;
  src?: string;
  hint?: string;
  fullSource?: string;
  notes?: string[];
  suggestions?: DiagSuggestion[];
}

export class BoruError extends Error {
  readonly code: string;
  readonly detail: string;
  readonly word: string;
  readonly row: number;
  readonly col: number;
  readonly src: string;
  readonly hint: string;
  readonly fullSource: string;
  readonly notes: string[];
  readonly suggestions: DiagSuggestion[];

  constructor(code: string, detail: string, word = "", metadata: BoruErrorMetadata = {}) {
    super(`[boru/${code}]: ${detail}`);
    this.name = "BoruError";
    this.code = code;
    this.detail = detail;
    this.word = word;
    this.row = metadata.row ?? 0;
    this.col = metadata.col ?? 0;
    this.src = metadata.src ?? word;
    this.hint = metadata.hint ?? "";
    this.fullSource = metadata.fullSource ?? "";
    this.notes = metadata.notes ?? [];
    this.suggestions = metadata.suggestions ?? [];
  }
}
