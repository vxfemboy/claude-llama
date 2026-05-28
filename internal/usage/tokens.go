// Package usage tracks the token savings produced by delegating work to the
// local llama. It does two jobs: estimate token counts (cheap, approximate)
// and append per-call rows to a JSONL log for later analysis.
package usage

// Estimate returns a back-of-envelope token count for s.
//
// We use the 4-chars-per-token heuristic that OpenAI and Anthropic both
// document as a reasonable approximation for English-ish text. It is not
// a tokenizer; it is honest about being a heuristic. The point is to give
// users a defensible "tokens saved" number, not a precise one.
func Estimate(s string) int {
	if len(s) == 0 {
		return 0
	}
	// Round up so even tiny strings register as 1 token; matches user intuition.
	return (len(s) + 3) / 4
}
