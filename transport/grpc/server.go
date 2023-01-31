package grpc

import (
	"context"
	"github.com/go-slark/slark/logger"
	"github.com/go-slark/slark/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"net"
)

type Server struct {
	*grpc.Server
	health   *health.Server
	listener net.Listener
	err      error
	logger   logger.Logger
	network  string
	address  string
	mw       []middleware.Middleware
	opts     []grpc.ServerOption
	unary    []grpc.UnaryServerInterceptor
	stream   []grpc.StreamServerInterceptor
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		network: "tcp",
		address: "0.0.0.0:0",
		health:  health.NewServer(),
	}
	for _, o := range opts {
		o(srv)
	}

	if len(srv.mw) == 0 {
		srv.mw = make([]middleware.Middleware, 0)
	}
	srv.mw = append(srv.mw, middleware.Validate(), middleware.Recovery(srv.logger))

	var grpcOpts []grpc.ServerOption
	srv.unary = append(srv.unary, srv.unaryServerInterceptor())
	if len(srv.unary) > 0 {
		grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(srv.unary...))
	}
	if len(srv.stream) > 0 {
		grpcOpts = append(grpcOpts, grpc.ChainStreamInterceptor(srv.stream...))
	}
	if len(srv.opts) > 0 {
		grpcOpts = append(grpcOpts, srv.opts...)
	}

	srv.Server = grpc.NewServer(grpcOpts...)
	srv.err = srv.listen()
	grpc_health_v1.RegisterHealthServer(srv.Server, srv.health)
	reflection.Register(srv.Server)
	return srv
}

func (s *Server) Start() error {
	if s.err != nil {
		return s.err
	}
	s.health.Resume()
	return s.Serve(s.listener)
}

func (s *Server) Stop(ctx context.Context) error {
	s.health.Shutdown()
	s.GracefulStop()
	return nil
}

func (s *Server) listen() error {
	l, err := net.Listen(s.network, s.address)
	if err != nil {
		return err
	}
	s.listener = l
	return nil
}

type ServerOption func(*Server)

func Network(network string) ServerOption {
	return func(s *Server) {
		s.network = network
	}
}

func Address(addr string) ServerOption {
	return func(s *Server) {
		s.address = addr
	}
}

func Listener(l net.Listener) ServerOption {
	return func(s *Server) {
		s.listener = l
	}
}

func Logger(logger logger.Logger) ServerOption {
	return func(server *Server) {
		server.logger = logger
	}
}

func UnaryInterceptor(u []grpc.UnaryServerInterceptor) ServerOption {
	return func(s *Server) {
		s.unary = u
	}
}

func StreamInterceptor(s []grpc.StreamServerInterceptor) ServerOption {
	return func(server *Server) {
		server.stream = s
	}
}

func ServerOptions(opts []grpc.ServerOption) ServerOption {
	return func(s *Server) {
		s.opts = opts
	}
}

func Middleware(mw []middleware.Middleware) ServerOption {
	return func(server *Server) {
		server.mw = mw
	}
}
