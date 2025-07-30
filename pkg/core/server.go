package core

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type Service interface {
	Name() string
	Start(ctx context.Context) *errors.Error
	Stop(ctx context.Context) *errors.Error
}

type ListeningService interface {
	Service
	AddTCP(addr string) *errors.Error
	AddTLS(addr, certFile, keyFile string) *errors.Error
	AddUDS(path string, mode *os.FileMode) *errors.Error
}

type BackgroundService interface {
	Service
	Run(ctx context.Context, shutdown <-chan struct{}) *errors.Error
}

type Server struct {
	services     []Service
	shutdownChan chan struct{}
	wg           sync.WaitGroup
	mu           sync.RWMutex
}

type ServerConfig struct {
	GracefulShutdownTimeout time.Duration
	PidFile                 string
	Daemon                  bool
	UpgradeSocket          string
	Workers                int
}

func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = &ServerConfig{
			GracefulShutdownTimeout: 30 * time.Second,
			Workers:                 1,
		}
	}
	
	return &Server{
		services:     make([]Service, 0),
		shutdownChan: make(chan struct{}),
	}
}

func (s *Server) AddService(service Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services = append(s.services, service)
}

func (s *Server) AddServices(services []Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services = append(s.services, services...)
}

func (s *Server) Bootstrap() *errors.Error {
	return nil
}

func (s *Server) RunForever() *errors.Error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.mu.RLock()
	services := make([]Service, len(s.services))
	copy(services, s.services)
	s.mu.RUnlock()

	for _, service := range services {
		s.wg.Add(1)
		go func(svc Service) {
			defer s.wg.Done()
			if err := svc.Start(ctx); err != nil {
				log.Printf("Service %s failed to start: %v", svc.Name(), err)
			}
		}(service)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down gracefully...", sig)
	case <-s.shutdownChan:
		log.Println("Shutdown requested")
	}

	return s.gracefulShutdown(ctx, services)
}

func (s *Server) Shutdown() {
	close(s.shutdownChan)
}

func (s *Server) gracefulShutdown(ctx context.Context, services []Service) *errors.Error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(svc Service) {
			defer wg.Done()
			if err := svc.Stop(shutdownCtx); err != nil {
				log.Printf("Service %s failed to stop gracefully: %v", svc.Name(), err)
			}
		}(service)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All services stopped gracefully")
	case <-shutdownCtx.Done():
		log.Println("Shutdown timeout reached, forcing exit")
		return errors.New(errors.TimeoutError, "graceful shutdown timeout")
	}

	return nil
}

type TCPKeepalive struct {
	Idle     time.Duration
	Interval time.Duration
	Count    int
}

type TCPSocketOptions struct {
	TCPFastOpen  *int
	TCPKeepalive *TCPKeepalive
	ReusePort    bool
	Backlog      int
}

func DefaultTCPSocketOptions() *TCPSocketOptions {
	return &TCPSocketOptions{
		Backlog: 128,
	}
}

type TLSSettings struct {
	CertFile     string
	KeyFile      string
	EnableH2     bool
	MinVersion   uint16
	MaxVersion   uint16
	CipherSuites []uint16
}

func (t *TLSSettings) EnableHTTP2() {
	t.EnableH2 = true
}

type HttpPeer struct {
	Addr    string
	TLS     bool
	SNI     string
	Options PeerOptions
}

type PeerOptions struct {
	HTTPVersion HTTPVersionPreference
	Timeout     time.Duration
}

type HTTPVersionPreference struct {
	HTTP2 int
	HTTP1 int
}

func (o *PeerOptions) SetHTTPVersion(http2Pref, http1Pref int) {
	o.HTTPVersion = HTTPVersionPreference{
		HTTP2: http2Pref,
		HTTP1: http1Pref,
	}
}

func NewHTTPPeer(addr string, tls bool, sni string) *HttpPeer {
	return &HttpPeer{
		Addr: addr,
		TLS:  tls,
		SNI:  sni,
		Options: PeerOptions{
			Timeout: 30 * time.Second,
			HTTPVersion: HTTPVersionPreference{
				HTTP2: 2,
				HTTP1: 1,
			},
		},
	}
}

type ServiceBuilder interface {
	Name() string
	Build() (Service, *errors.Error)
}

type HTTPServiceBuilder struct {
	name     string
	handlers map[string]func(context.Context) *errors.Error
}

func NewHTTPServiceBuilder(name string) *HTTPServiceBuilder {
	return &HTTPServiceBuilder{
		name:     name,
		handlers: make(map[string]func(context.Context) *errors.Error),
	}
}

func (b *HTTPServiceBuilder) Name() string {
	return b.name
}

func (b *HTTPServiceBuilder) Build() (Service, *errors.Error) {
	return &httpService{
		name:     b.name,
		handlers: b.handlers,
	}, nil
}

type httpService struct {
	name     string
	handlers map[string]func(context.Context) *errors.Error
	listener net.Listener
}

func (s *httpService) Name() string {
	return s.name
}

func (s *httpService) Start(ctx context.Context) *errors.Error {
	return nil
}

func (s *httpService) Stop(ctx context.Context) *errors.Error {
	if s.listener != nil {
		return errors.OrErr(s.listener.Close(), errors.InternalError, "failed to close listener")
	}
	return nil
}

type Connector interface {
	GetHTTPSession(peer *HttpPeer) (HTTPSession, bool, *errors.Error)
	ReleaseHTTPSession(session HTTPSession, peer *HttpPeer, timeout *time.Duration) *errors.Error
}

type HTTPSession interface {
	WriteRequestHeader(header interface{}) *errors.Error
	FinishRequestBody() *errors.Error
	ReadResponseHeader() *errors.Error
	ResponseHeader() interface{}
	ReadResponseBody() ([]byte, *errors.Error)
	Close() *errors.Error
}