package router

import (
	"context"
	"testing"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/session"
	json "github.com/goccy/go-json"
	"github.com/rs/zerolog"
)

func init() {
	// Disable logging during benchmarks
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

func BenchmarkParseRequest(b *testing.B) {
	parser := NewParser()
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"/tmp/test.txt"}}}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouteMessage(b *testing.B) {
	r := NewRouter()

	// Set up mock handlers
	r.SetUpstreamSender(func(ctx context.Context, msg []byte) ([]byte, error) {
		return msg, nil
	})
	r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
		return &PolicyDecision{Allow: true, MatchedRule: "test"}, nil
	})
	r.SetAuditLogger(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext, decision *PolicyDecision, response []byte, latency time.Duration) {
		// No-op for benchmark
	})

	sess := session.NewSession("bench-session")
	sess.SetAgent("bench-agent", "Bench Agent", []string{"read:*"})

	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"/tmp/test.txt"}}}`)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := r.Route(ctx, sess, msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateRequestID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = generateRequestID()
	}
}

func BenchmarkNewRequestContext(b *testing.B) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test"}`),
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := NewRequestContext(req)
		ctx.Release()
	}
}

func BenchmarkResponseBuilder(b *testing.B) {
	builder := NewResponseBuilder()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp := builder.Success(1, map[string]interface{}{"result": "ok"})
		_, _ = builder.Marshal(resp)
	}
}
