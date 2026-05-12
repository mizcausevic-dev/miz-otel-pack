package otelpack

// Pricing is the per-million-token cost in USD for a single model,
// broken into input and output prices.
type Pricing struct {
	Provider     string
	Model        string
	InputPerMTok float64
	OutputPerMTok float64
}

// pricingTable is an opinionated, easily-overridable model price list.
// Values are rough public list prices as of late 2025; consumers SHOULD
// override with their actual negotiated rates via RegisterPricing.
var pricingTable = map[string]Pricing{
	"claude-opus-4-7":     {Provider: "Anthropic", Model: "claude-opus-4-7", InputPerMTok: 15.00, OutputPerMTok: 75.00},
	"claude-sonnet-4-6":   {Provider: "Anthropic", Model: "claude-sonnet-4-6", InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-haiku-4-5":    {Provider: "Anthropic", Model: "claude-haiku-4-5", InputPerMTok: 0.80, OutputPerMTok: 4.00},
	"gpt-4o-2024-08-06":   {Provider: "OpenAI", Model: "gpt-4o-2024-08-06", InputPerMTok: 2.50, OutputPerMTok: 10.00},
	"gpt-4o-mini":         {Provider: "OpenAI", Model: "gpt-4o-mini", InputPerMTok: 0.15, OutputPerMTok: 0.60},
	"gemini-1.5-pro":      {Provider: "Google", Model: "gemini-1.5-pro", InputPerMTok: 1.25, OutputPerMTok: 5.00},
	"gemini-1.5-flash":    {Provider: "Google", Model: "gemini-1.5-flash", InputPerMTok: 0.075, OutputPerMTok: 0.30},
}

// RegisterPricing adds or overrides a model price. Returns the previous value (if any).
func RegisterPricing(p Pricing) Pricing {
	old := pricingTable[p.Model]
	pricingTable[p.Model] = p
	return old
}

// LookupPricing returns the registered price for a model, or zero values if unknown.
func LookupPricing(model string) (Pricing, bool) {
	p, ok := pricingTable[model]
	return p, ok
}

// CostUSD computes the dollar cost of a request given the model and token counts.
// Returns 0 if the model is not in the pricing table.
func CostUSD(model string, inputTokens, outputTokens int64) float64 {
	p, ok := pricingTable[model]
	if !ok {
		return 0
	}
	return (float64(inputTokens)/1_000_000)*p.InputPerMTok +
		(float64(outputTokens)/1_000_000)*p.OutputPerMTok
}
