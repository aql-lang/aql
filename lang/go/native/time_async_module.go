package native

// TimeAsyncModuleNatives holds the clock / timer / async words that were moved
// out of the core registry into the aql:time-util module (TimeUtil namespace):
//
//	now       the current instant (host clock)
//	sleep     block for N milliseconds
//	timeout   run a body once after a delay (returns a Timeout handle)
//	interval  run a body repeatedly every N ms (returns an Interval handle)
//	await     wait for a timer/async body to complete
//	cancel    cancel a Timeout or Interval
//
// The handlers stay in their feature files (natives.go / native_misc.go) in
// this package; only modules.BuildTimeModule registers these, so they are no
// longer available unqualified. At dispatch the module wrapper short-circuits
// to the inner handler against the live registry (host clock, timers).
var TimeAsyncModuleNatives = []NativeFunc{
	{
		Name: "now",
		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: nowHandler,
			Returns: []*Type{TInstant}, BarrierPos: 0,
		}},
	},
	{
		Name: "sleep",
		Signatures: []NativeSig{{
			Args:    []*Type{TInteger},
			Handler: sleepHandler,
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
	{
		Name: "timeout",
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TList}, QuoteArgs: map[int]bool{1: true}, Handler: timeoutListHandler, Returns: []*Type{TTimeout}, BarrierPos: -1},
			{Args: []*Type{TInteger, TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: timeoutWordHandler, Returns: []*Type{TTimeout}, BarrierPos: -1},
		},
	},
	{
		Name: "interval",
		Signatures: []NativeSig{
			{Args: []*Type{TInteger, TList}, QuoteArgs: map[int]bool{1: true}, Handler: intervalListHandler, Returns: []*Type{TInterval}, BarrierPos: -1},
			{Args: []*Type{TInteger, TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: intervalAtomHandler, Returns: []*Type{TInterval}, BarrierPos: -1},
		},
	},
	{
		Name: "await",
		Signatures: []NativeSig{
			{Args: []*Type{TOptions, TList}, NoEvalArgs: map[int]bool{1: true}, Handler: awaitWithOptsHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TList}, NoEvalArgs: map[int]bool{0: true}, Handler: awaitDefaultHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "cancel",
		Signatures: []NativeSig{
			{Args: []*Type{TTimeout}, Handler: cancelTimeoutHandler, Returns: []*Type{}, BarrierPos: -1},
			{Args: []*Type{TInterval}, Handler: cancelIntervalHandler, Returns: []*Type{}, BarrierPos: -1},
		},
	},
}
