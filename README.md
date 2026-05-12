# miz-otel-pack

OpenTelemetry pack that translates **agent operation spans into business-metric spans**.

Drop it into any Go service running an OTel `TracerProvider`. Every span that carries the [OpenTelemetry GenAI semantic-convention attributes](https://opentelemetry.io/docs/specs/semconv/gen-ai/) (`gen_ai.system`, `gen_ai.request.model`, `gen_ai.usage.input_tokens`, etc.) automatically gets a **sibling business span** stamped with `business.cost_usd`, `business.tokens_total`, `business.latency_ms`, `business.tenant`, and `business.model` — the attributes finance and product teams actually want to slice by.

Zero changes to your application's instrumentation. The translation happens at the SpanProcessor layer.

## Install

```bash
go get github.com/mizcausevic-dev/miz-otel-pack
```

## Quickstart

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/sdk/trace"
    otelpack "github.com/mizcausevic-dev/miz-otel-pack"
)

func setupTracing(exporter trace.SpanExporter) *trace.TracerProvider {
    underlying := trace.NewBatchSpanProcessor(exporter)
    biz := otelpack.NewBusinessSpanProcessor(underlying, "agent-service")
    tp := trace.NewTracerProvider(trace.WithSpanProcessor(biz))
    otel.SetTracerProvider(tp)
    return tp
}
```

That's it. Now whenever any code in your service emits a GenAI span:

```go
ctx, span := tracer.Start(ctx, "llm.invoke")
span.SetAttributes(
    attribute.String("gen_ai.system", "Anthropic"),
    attribute.String("gen_ai.request.model", "claude-sonnet-4-6"),
    attribute.Int64("gen_ai.usage.input_tokens", 1000),
    attribute.Int64("gen_ai.usage.output_tokens", 500),
    attribute.String("gen_ai.user.tenant", "tnt_acme"),
)
// ... do the work
span.End()
```

…a sibling `business.llm.invoke` span is automatically emitted with:

| Attribute | Value |
|---|---|
| `business.cost_usd` | `0.0105` (1000 × $3/M + 500 × $15/M) |
| `business.tokens_in` | `1000` |
| `business.tokens_out` | `500` |
| `business.tokens_total` | `1500` |
| `business.latency_ms` | `42` |
| `business.tenant` | `tnt_acme` |
| `business.provider` | `Anthropic` |
| `business.model` | `claude-sonnet-4-6` |
| `business.source_span_id` | (link back to the original GenAI span) |

## Overriding pricing

Built-in price list covers the major Claude / OpenAI / Gemini SKUs at rough public list prices. Override with your actual negotiated rates:

```go
otelpack.RegisterPricing(otelpack.Pricing{
    Provider: "Anthropic",
    Model: "claude-sonnet-4-6",
    InputPerMTok: 2.40,   // your enterprise rate
    OutputPerMTok: 12.00,
})
```

## Why bother?

Your existing OTel pipeline already collects GenAI spans. Without this pack, finance and product teams have to write bespoke queries to join token counts with model price lists, summed across tenants, partitioned by feature. With this pack, they just `business.cost_usd` from the same observability tool everyone else uses.

## Compatibility

- OpenTelemetry SDK for Go `v1.32+`
- Go `1.22+`
- Works with any `trace.SpanExporter` (OTLP, Jaeger, Zipkin, stdout, etc.)

## Development

```bash
go vet ./...
go test -race -v ./...
go build ./...
```

## License

AGPL-3.0.

---

**Connect:** [LinkedIn](https://www.linkedin.com/in/mirzacausevic/) · [Kinetic Gain](https://kineticgain.com) · [Medium](https://medium.com/@mizcausevic/) · [Skills](https://mizcausevic.com/skills/)
