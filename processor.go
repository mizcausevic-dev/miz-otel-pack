// Package otelpack provides an OpenTelemetry SpanProcessor that detects
// "agent operation" spans (using the OpenTelemetry GenAI semantic
// conventions) and emits derived business-metric spans alongside them.
//
// The package layers on top of an existing TracerProvider — install it
// once, and every GenAI span automatically gets a sibling business span
// with cost_usd, tokens_total, latency_ms, and tenant attributes.
package otelpack

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	otrace "go.opentelemetry.io/otel/trace"
)

// OpenTelemetry GenAI semantic-convention attribute keys.
// See https://opentelemetry.io/docs/specs/semconv/gen-ai/.
const (
	AttrGenAISystem        = "gen_ai.system"
	AttrGenAIRequestModel  = "gen_ai.request.model"
	AttrGenAIInputTokens   = "gen_ai.usage.input_tokens"
	AttrGenAIOutputTokens  = "gen_ai.usage.output_tokens"
	AttrGenAITenant        = "gen_ai.user.tenant"

	// Business-metric attributes emitted on the derived span.
	BizAttrCostUSD      = "business.cost_usd"
	BizAttrTokensIn     = "business.tokens_in"
	BizAttrTokensOut    = "business.tokens_out"
	BizAttrTokensTotal  = "business.tokens_total"
	BizAttrLatencyMs    = "business.latency_ms"
	BizAttrTenant       = "business.tenant"
	BizAttrModel        = "business.model"
	BizAttrProvider     = "business.provider"
	BizAttrParentSpanID = "business.source_span_id"
)

// BusinessSpanProcessor wraps an OTel SpanProcessor. When a span ends
// that carries GenAI semantic-convention attributes, the processor
// emits a sibling span tagged `business.*` on the configured tracer.
type BusinessSpanProcessor struct {
	next       tracesdk.SpanProcessor
	tracerName string

	mu     sync.RWMutex
	closed bool
}

// NewBusinessSpanProcessor wraps `next` and emits business spans onto
// the named tracer. If tracerName is empty, the default tracer
// "miz-otel-pack" is used.
//
// The tracer is resolved lazily on each OnEnd call via the global
// TracerProvider, so it always reflects the most recently set provider.
func NewBusinessSpanProcessor(next tracesdk.SpanProcessor, tracerName string) *BusinessSpanProcessor {
	if tracerName == "" {
		tracerName = "miz-otel-pack"
	}
	return &BusinessSpanProcessor{
		next:       next,
		tracerName: tracerName,
	}
}

// OnStart is forwarded to the wrapped processor.
func (p *BusinessSpanProcessor) OnStart(parent context.Context, s tracesdk.ReadWriteSpan) {
	p.next.OnStart(parent, s)
}

// OnEnd inspects the span. If it carries GenAI attributes, a derived
// business span is emitted before forwarding to the wrapped processor.
func (p *BusinessSpanProcessor) OnEnd(s tracesdk.ReadOnlySpan) {
	p.emitBusinessSpan(s)
	p.next.OnEnd(s)
}

// Compile-time interface check.
var _ tracesdk.SpanProcessor = (*BusinessSpanProcessor)(nil)

// Shutdown forwards to the wrapped processor.
func (p *BusinessSpanProcessor) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return p.next.Shutdown(ctx)
}

// ForceFlush forwards to the wrapped processor.
func (p *BusinessSpanProcessor) ForceFlush(ctx context.Context) error {
	return p.next.ForceFlush(ctx)
}

func (p *BusinessSpanProcessor) emitBusinessSpan(s tracesdk.ReadOnlySpan) {
	attrs := s.Attributes()
	model := stringAttr(attrs, AttrGenAIRequestModel)
	if model == "" {
		return // not a GenAI span
	}

	inputTokens := intAttr(attrs, AttrGenAIInputTokens)
	outputTokens := intAttr(attrs, AttrGenAIOutputTokens)
	tenant := stringAttr(attrs, AttrGenAITenant)
	provider := stringAttr(attrs, AttrGenAISystem)
	if provider == "" {
		if pricing, ok := LookupPricing(model); ok {
			provider = pricing.Provider
		}
	}

	costUSD := CostUSD(model, inputTokens, outputTokens)
	latencyMs := s.EndTime().Sub(s.StartTime()).Milliseconds()

	bizAttrs := []attribute.KeyValue{
		attribute.Float64(BizAttrCostUSD, costUSD),
		attribute.Int64(BizAttrTokensIn, inputTokens),
		attribute.Int64(BizAttrTokensOut, outputTokens),
		attribute.Int64(BizAttrTokensTotal, inputTokens+outputTokens),
		attribute.Int64(BizAttrLatencyMs, latencyMs),
		attribute.String(BizAttrTenant, tenant),
		attribute.String(BizAttrModel, model),
		attribute.String(BizAttrProvider, provider),
		attribute.String(BizAttrParentSpanID, s.SpanContext().SpanID().String()),
	}

	tracer := otel.Tracer(p.tracerName)
	_, span := tracer.Start(
		context.Background(),
		"business."+s.Name(),
		otrace.WithTimestamp(s.StartTime()),
		otrace.WithAttributes(bizAttrs...),
	)
	span.End(otrace.WithTimestamp(s.EndTime()))
}

func stringAttr(attrs []attribute.KeyValue, key string) string {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

func intAttr(attrs []attribute.KeyValue, key string) int64 {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv.Value.AsInt64()
		}
	}
	return 0
}

