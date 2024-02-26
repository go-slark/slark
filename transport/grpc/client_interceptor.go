package grpc

import (
	"context"
	"github.com/go-slark/slark/middleware"
	utils "github.com/go-slark/slark/pkg"
	tracing "github.com/go-slark/slark/pkg/trace"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"strconv"
	"time"
)

// trace -> metric -> breaker -> timeout -> ...

func unaryClientInterceptor(mw ...middleware.Middleware) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := middleware.ComposeMiddleware(mw...)(func(ctx context.Context, req interface{}) (interface{}, error) {
			return reply, invoker(ctx, method, req, reply, cc, opts...)
		})(ctx, req)
		return err
	}
}

func streamClientInterceptor(mw []middleware.Middleware) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		rsp, err := middleware.ComposeMiddleware(mw...)(func(ctx context.Context, req interface{}) (interface{}, error) {
			return streamer(ctx, desc, cc, method, opts...)
		})(ctx, nil)
		// TODO
		return rsp.(grpc.ClientStream), err
	}
}

func ClientTimeout(time time.Duration) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			var cancel context.CancelFunc
			if _, ok := ctx.Deadline(); !ok && time > 0 {
				ctx, cancel = context.WithTimeout(ctx, time)
			}
			if cancel != nil {
				defer cancel()
			}
			return handler(ctx, req)
		}
	}
}

func ClientRayID() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			value := ctx.Value(utils.RayID)
			requestID, ok := value.(string)
			if !ok || len(requestID) == 0 {
				requestID = utils.BuildRequestID()
			}
			ctx = metadata.AppendToOutgoingContext(ctx, utils.RayID, requestID)
			return handler(ctx, req)
		}
	}
}

func ClientAuthZ() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			token, ok := ctx.Value(utils.Token).(string)
			if ok {
				ctx = metadata.AppendToOutgoingContext(ctx, utils.Token, strconv.QuoteToASCII(token))
			}
			return handler(ctx, req)
		}
	}
}

func UnaryClientTrace(opts ...tracing.Option) grpc.UnaryClientInterceptor {
	tracer := tracing.NewTracer(trace.SpanKindClient, opts...)
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		name, attrs := attribute(ctx, cc.Target())
		opt := []trace.SpanStartOption{
			trace.WithSpanKind(tracer.Kind()),
			trace.WithAttributes(attrs...),
		}
		ctx, span := tracer.Start(ctx, name, &tracing.Carrier{MD: md}, opt...)
		defer span.End()
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			s, _ := status.FromError(err)
			span.SetStatus(codes.Error, s.Message())
			span.SetAttributes(semconv.RPCGRPCStatusCodeKey.String(s.Code().String()))
			return err
		}
		span.SetAttributes(semconv.RPCGRPCStatusCodeKey.String(grpccodes.OK.String()))
		return nil
	}
}

type clientStreamWrapper struct {
	grpc.ClientStream
	rMsgID int
	sMsgID int
}

func (w *clientStreamWrapper) RecvMsg(m interface{}) error {
	err := w.ClientStream.RecvMsg(m)
	if err != nil {
		return err
	}
	w.rMsgID++
	return nil
}

func (w *clientStreamWrapper) SendMsg(m interface{}) error {
	w.sMsgID++
	return w.ClientStream.SendMsg(m)
}

func StreamClientTrace(opts ...tracing.Option) grpc.StreamClientInterceptor {
	tracer := tracing.NewTracer(trace.SpanKindClient, opts...)
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		name, attrs := attribute(ctx, cc.Target())
		opt := []trace.SpanStartOption{
			trace.WithSpanKind(tracer.Kind()),
			trace.WithAttributes(attrs...),
		}
		ctx, span := tracer.Start(ctx, name, &tracing.Carrier{MD: md}, opt...)
		defer span.End()
		s, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			grpcStatus, _ := status.FromError(err)
			span.SetStatus(codes.Error, grpcStatus.Message())
			span.SetAttributes(semconv.RPCGRPCStatusCodeKey.String(grpcStatus.Code().String()))
			return s, err
		}
		stream := &clientStreamWrapper{ClientStream: s}
		return stream, nil
	}
}