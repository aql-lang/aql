package eng

// quote_lambda_screen_test.go pins the quote-polarity screen
// (checker-compiler-completeness-review.0.md §2.2): an Atom-typed lambda
// param is a quote-capture slot the runtime never binds from a delivered
// stack value, so (a) lambdaHookCompatible must decline such a lambda as a
// HOF callback, and (b) quoteParamCarrierBind must report a matched sig
// binding a computed carrier to such a slot (the closure-unit guard that
// declines the check-mode apply the runtime rejects).
