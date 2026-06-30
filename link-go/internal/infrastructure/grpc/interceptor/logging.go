package interceptor

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

type LoggingConfig struct {
	Logger      *logrus.Logger
	LogRequest  bool
	LogResponse bool
	LogPayload  bool
}

func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Logger:      logrus.StandardLogger(),
		LogRequest:  true,
		LogResponse: true,
		LogPayload:  false,
	}
}

func LoggingUnary(cfg *LoggingConfig) grpc.UnaryClientInterceptor {
	if cfg == nil {
		cfg = DefaultLoggingConfig()
	}
	logger := cfg.Logger.WithField("component", "grpc-client")

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		peerAddr := ""
		if p, ok := peer.FromContext(ctx); ok {
			peerAddr = p.Addr.String()
		}

		if cfg.LogRequest {
			fields := logrus.Fields{"method": method, "peer": peerAddr, "type": "unary"}
			if cfg.LogPayload {
				fields["request"] = req
			}
			logger.WithFields(fields).Debug("gRPC request")
		}

		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		if cfg.LogResponse {
			fields := logrus.Fields{"method": method, "peer": peerAddr, "duration": duration, "type": "unary"}
			if err != nil {
				fields["error"] = err.Error()
				logger.WithFields(fields).Error("gRPC response")
			} else {
				if cfg.LogPayload {
					fields["response"] = reply
				}
				logger.WithFields(fields).Debug("gRPC response")
			}
		}
		return err
	}
}

func LoggingStream(cfg *LoggingConfig) grpc.StreamClientInterceptor {
	if cfg == nil {
		cfg = DefaultLoggingConfig()
	}
	logger := cfg.Logger.WithField("component", "grpc-client")

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		start := time.Now()
		peerAddr := ""
		if p, ok := peer.FromContext(ctx); ok {
			peerAddr = p.Addr.String()
		}

		if cfg.LogRequest {
			logger.WithFields(logrus.Fields{
				"method": method, "peer": peerAddr, "type": "stream",
				"server_stream": desc.ServerStreams, "client_stream": desc.ClientStreams,
			}).Debug("gRPC stream start")
		}

		stream, err := streamer(ctx, desc, cc, method, opts...)
		duration := time.Since(start)

		if cfg.LogResponse {
			fields := logrus.Fields{"method": method, "peer": peerAddr, "duration": duration}
			if err != nil {
				fields["error"] = err.Error()
				logger.WithFields(fields).Error("gRPC stream failed")
			} else {
				logger.WithFields(fields).Debug("gRPC stream established")
			}
		}
		return stream, err
	}
}
