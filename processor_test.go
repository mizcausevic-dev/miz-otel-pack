package otelpack

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Note: trace is the sdk/trace package; we don't need WithTimestamp in tests.

// recordingProcessor captures every OnEnd call so tests can inspect spans.
type recordingProcessor struct {
	mu    sync.Mutex
	ended []trace.ReadOnlySpan
}

func (r *recordingProcessor) OnStart(_ context.Context, _ trace.ReadWriteSpan) {}
func (r *recordingProcessor) OnEnd(s trace.ReadOnlySpan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, s)
}
func (r *recordingProcessor) Shutdown(_ context.Context) error  { return nil }
func (r *recordingProcessor) ForceFlush(_ context.Context) error { return nil }

func (r *recordingProcessor) findBy(t *testing.T, name string) trace.ReadOnlySpan {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.ended {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

func setupProvider(t *testing.T) (*trace.TracerProvider, *recordingProcessor) {
	t.Helper()
	rec := &recordingProcessor{}
	wrapped := NewBusinessSpanProcessor(rec, "test-tracer")
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(wrapped))
	otel.SetTracerProvider(tp)
	return tp, rec
}

func attrValue(attrs []attribute.KeyValue, key string) attribute.Value {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv.Value
		}
	}
	return attribute.Value{}
}

func TestEmitsBusinessSpanForGenAIOperation(t *testing.T) {
	tp, rec := setupProvider(t)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("agent")
	_, span := tracer.Start(context.Background(), "llm.invoke")
	span.SetAttributes(
		attribute.String(AttrGenAISystem, "Anthropic"),
		attribute.String(AttrGenAIRequestModel, "claude-sonnet-4-6"),
		attribute.Int64(AttrGenAIInputTokens, 1000),
		attribute.Int64(AttrGenAIOutputTokens, 500),
		attribute.String(AttrGenAITenant, "tnt_acme"),
	)
	time.Sleep(2 * time.Millisecond)
	span.End()

	biz := rec.findBy(t, "business.llm.invoke")
	if biz == nil {
		t.Fatal("business span not emitted")
	}

	cost := attrValue(biz.Attributes(), BizAttrCostUSD).AsFloat64()
	// 1000 in @ $3/M + 500 out @ $15/M = 0.003 + 0.0075 = 0.0105
	if cost <= 0.0104 || cost >= 0.0106 {
		t.Errorf("cost = %v, want ~0.0105", cost)
	}
	if v := attrValue(biz.Attributes(), BizAttrTokensTotal).AsInt64(); v != 1500 {
		t.Errorf("tokens_total = %d, want 1500", v)
	}
	if v := attrValue(biz.Attributes(), BizAttrTenant).AsString(); v != "tnt_acme" {
		t.Errorf("tenant = %q, want tnt_acme", v)
	}
	if v := attrValue(biz.Attributes(), BizAttrProvider).AsString(); v != "Anthropic" {
		t.Errorf("provider = %q, want Anthropic", v)
	}
}

func TestSkipsSpansWithoutGenAIAttributes(t *testing.T) {
	tp, rec := setupProvider(t)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("agent")
	_, span := tracer.Start(context.Background(), "ordinary.operation")
	span.End()

	if got := rec.findBy(t, "business.ordinary.operation"); got != nil {
		t.Error("business span should NOT be emitted for non-GenAI spans")
	}
}

func TestUsesPricingTableWhenProviderUnset(t *testing.T) {
	tp, rec := setupProvider(t)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("agent")
	_, span := tracer.Start(context.Background(), "llm.invoke")
	span.SetAttributes(
		// Note: no gen_ai.system attribute set
		attribute.String(AttrGenAIRequestModel, "claude-opus-4-7"),
		attribute.Int64(AttrGenAIInputTokens, 100),
		attribute.Int64(AttrGenAIOutputTokens, 100),
	)
	span.End()

	biz := rec.findBy(t, "business.llm.invoke")
	if biz == nil {
		t.Fatal("business span not emitted")
	}
	if v := attrValue(biz.Attributes(), BizAttrProvider).AsString(); v != "Anthropic" {
		t.Errorf("provider should be derived from pricing table; got %q", v)
	}
}

func TestCostUSDUnknownModel(t *testing.T) {
	if got := CostUSD("mystery-model-9000", 1000, 500); got != 0 {
		t.Errorf("CostUSD for unknown model = %v, want 0", got)
	}
}

func TestRegisterPricingOverride(t *testing.T) {
	old := RegisterPricing(Pricing{Model: "custom-model", InputPerMTok: 2.0, OutputPerMTok: 8.0})
	defer RegisterPricing(old) // restore

	cost := CostUSD("custom-model", 1_000_000, 1_000_000)
	if cost != 10.0 {
		t.Errorf("custom cost = %v, want 10.0", cost)
	}
}

func TestLookupPricingForKnownModels(t *testing.T) {
	models := []string{"claude-opus-4-7", "claude-sonnet-4-6", "gpt-4o-2024-08-06", "gemini-1.5-pro"}
	for _, m := range models {
		if _, ok := LookupPricing(m); !ok {
			t.Errorf("LookupPricing(%q) missing", m)
		}
	}
}
