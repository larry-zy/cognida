package interceptor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const InstrumentationName = "link/internal/infrastructure/grpc"

type TracerConfig struct {
	Tracer      trace.Tracer
	SpanOptions []trace.SpanStartOption
}

func DefaultTracerConfig() *TracerConfig {
	return &TracerConfig{Tracer: otel.Tracer(InstrumentationName)}
}

func UnaryTracing(cfg *TracerConfig) grpc.UnaryClientInterceptor {
	if cfg == nil {
		cfg = DefaultTracerConfig()
	}
	if cfg.Tracer == nil {
		cfg.Tracer = otel.Tracer(InstrumentationName)
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		attrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", cc.Target()),
			attribute.String("rpc.method", method),
			attribute.String("net.peer.name", cc.Target()),
		}
		startOpts := append(cfg.SpanOptions, trace.WithAttributes(attrs...))
		ctx, span := cfg.Tracer.Start(ctx, method, startOpts...)

		if req != nil {
			span.SetAttributes(attribute.String("rpc.request.type", reqTypeString(req)))
		}

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
			if reply != nil {
				span.SetAttributes(attribute.String("rpc.response.type", replyTypeString(reply)))
			}
		}
		span.End()
		return err
	}
}

func StreamTracing(cfg *TracerConfig) grpc.StreamClientInterceptor {
	if cfg == nil {
		cfg = DefaultTracerConfig()
	}
	if cfg.Tracer == nil {
		cfg.Tracer = otel.Tracer(InstrumentationName)
	}

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		attrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", cc.Target()),
			attribute.String("rpc.method", method),
			attribute.String("net.peer.name", cc.Target()),
			attribute.Bool("rpc.server_stream", desc.ServerStreams),
			attribute.Bool("rpc.client_stream", desc.ClientStreams),
		}
		startOpts := append(cfg.SpanOptions, trace.WithAttributes(attrs...))
		ctx, span := cfg.Tracer.Start(ctx, method, startOpts...)

		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return stream, err
		}

		span.SetStatus(codes.Ok, "")
		return &tracedStream{ClientStream: stream, span: span}, nil
	}
}

type tracedStream struct {
	grpc.ClientStream
	span trace.Span
}

func (s *tracedStream) Header() (metadata.MD, error) {
	md, err := s.ClientStream.Header()
	if err != nil {
		s.recordError(err)
	}
	return md, err
}

func (s *tracedStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		s.recordError(err)
	}
	return err
}

func (s *tracedStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.recordError(err)
	}
	return err
}

func (s *tracedStream) CloseSend() error {
	s.span.End()
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.recordError(err)
	}
	return err
}

func (s *tracedStream) Context() context.Context {
	return s.ClientStream.Context()
}

func (s *tracedStream) Trailer() metadata.MD {
	return s.ClientStream.Trailer()
}

func (s *tracedStream) recordError(err error) {
	if err != nil && s.span != nil {
		s.span.RecordError(err)
	}
}

func OtelUnaryInterceptor(opts ...otelgrpc.Option) grpc.UnaryClientInterceptor {
	return otelgrpc.UnaryClientInterceptor(opts...)
}

func OtelStreamInterceptor(opts ...otelgrpc.Option) grpc.StreamClientInterceptor {
	return otelgrpc.StreamClientInterceptor(opts...)
}

func reqTypeString(req interface{}) string {
	if req == nil {
		return "nil"
	}
	return typeString(req)
}

func replyTypeString(reply interface{}) string {
	if reply == nil {
		return "nil"
	}
	return typeString(reply)
}

func typeString(v interface{}) string {
	if v == nil {
		return "nil"
	}
	t := fmt.Sprintf("%T", v)
	if len(t) > 50 {
		return t[:50] + "..."
	}
	return t
}
