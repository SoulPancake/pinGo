package tls

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type TLSSettings struct {
	CertFile         string
	KeyFile          string
	CAFile           string
	ClientAuth       tls.ClientAuthType
	MinVersion       uint16
	MaxVersion       uint16
	CipherSuites     []uint16
	CurvePreferences []tls.CurveID
	EnableH2         bool
	SNICallback      func(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
}

func NewTLSSettings(certFile, keyFile string) (*TLSSettings, *errors.Error) {
	return &TLSSettings{
		CertFile:   certFile,
		KeyFile:    keyFile,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		ClientAuth: tls.NoClientCert,
	}, nil
}

func IntermediateTLS(certFile, keyFile string) (*TLSSettings, *errors.Error) {
	return &TLSSettings{
		CertFile:   certFile,
		KeyFile:    keyFile,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		ClientAuth: tls.NoClientCert,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
	}, nil
}

func ModernTLS(certFile, keyFile string) (*TLSSettings, *errors.Error) {
	return &TLSSettings{
		CertFile:   certFile,
		KeyFile:    keyFile,
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		ClientAuth: tls.NoClientCert,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
	}, nil
}

func (t *TLSSettings) EnableHTTP2() {
	t.EnableH2 = true
}

func (t *TLSSettings) SetClientAuth(auth tls.ClientAuthType) {
	t.ClientAuth = auth
}

func (t *TLSSettings) SetCAFile(caFile string) {
	t.CAFile = caFile
}

func (t *TLSSettings) BuildTLSConfig() (*tls.Config, *errors.Error) {
	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, errors.NewWithCause(errors.TLSError, "failed to load certificate", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   t.MinVersion,
		MaxVersion:   t.MaxVersion,
		ClientAuth:   t.ClientAuth,
	}

	if len(t.CipherSuites) > 0 {
		config.CipherSuites = t.CipherSuites
	}

	if len(t.CurvePreferences) > 0 {
		config.CurvePreferences = t.CurvePreferences
	}

	if t.EnableH2 {
		config.NextProtos = []string{"h2", "http/1.1"}
	} else {
		config.NextProtos = []string{"http/1.1"}
	}

	if t.CAFile != "" {
		caCert, err := ioutil.ReadFile(t.CAFile)
		if err != nil {
			return nil, errors.NewWithCause(errors.TLSError, "failed to read CA file", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New(errors.TLSError, "failed to parse CA certificate")
		}

		config.ClientCAs = caCertPool
	}

	if t.SNICallback != nil {
		config.GetCertificate = t.SNICallback
	}

	return config, nil
}

type TLSListener struct {
	listener net.Listener
	config   *tls.Config
}

func NewTLSListener(listener net.Listener, settings *TLSSettings) (*TLSListener, *errors.Error) {
	config, err := settings.BuildTLSConfig()
	if err != nil {
		return nil, err
	}

	return &TLSListener{
		listener: tls.NewListener(listener, config),
		config:   config,
	}, nil
}

func (l *TLSListener) Accept() (net.Conn, error) {
	return l.listener.Accept()
}

func (l *TLSListener) Close() error {
	return l.listener.Close()
}

func (l *TLSListener) Addr() net.Addr {
	return l.listener.Addr()
}

type TLSClient struct {
	config *tls.Config
	client *http.Client
}

type TLSClientConfig struct {
	InsecureSkipVerify bool
	ServerName         string
	RootCAs            *x509.CertPool
	ClientCertificates []tls.Certificate
	MinVersion         uint16
	MaxVersion         uint16
}

func NewTLSClient(config *TLSClientConfig) *TLSClient {
	if config == nil {
		config = &TLSClientConfig{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkipVerify,
		ServerName:         config.ServerName,
		RootCAs:            config.RootCAs,
		Certificates:       config.ClientCertificates,
		MinVersion:         config.MinVersion,
		MaxVersion:         config.MaxVersion,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &TLSClient{
		config: tlsConfig,
		client: client,
	}
}

func (c *TLSClient) GetClient() *http.Client {
	return c.client
}

func (c *TLSClient) GetConfig() *tls.Config {
	return c.config
}

type CertificateManager struct {
	certificates map[string]*tls.Certificate
	mu           sync.RWMutex
}

func NewCertificateManager() *CertificateManager {
	return &CertificateManager{
		certificates: make(map[string]*tls.Certificate),
	}
}

func (cm *CertificateManager) AddCertificate(hostname, certFile, keyFile string) *errors.Error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return errors.NewWithCause(errors.TLSError, "failed to load certificate", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.certificates[hostname] = &cert

	return nil
}

func (cm *CertificateManager) RemoveCertificate(hostname string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.certificates, hostname)
}

func (cm *CertificateManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cert, exists := cm.certificates[hello.ServerName]; exists {
		return cert, nil
	}

	if cert, exists := cm.certificates["*"]; exists {
		return cert, nil
	}

	return nil, errors.New(errors.TLSError, "no certificate found for hostname")
}

func (cm *CertificateManager) ListCertificates() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	hostnames := make([]string, 0, len(cm.certificates))
	for hostname := range cm.certificates {
		hostnames = append(hostnames, hostname)
	}

	return hostnames
}

func ValidateCertificate(certFile, keyFile string) *errors.Error {
	_, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return errors.NewWithCause(errors.TLSError, "invalid certificate/key pair", err)
	}
	return nil
}

func GetCertificateInfo(certFile string) (*x509.Certificate, *errors.Error) {
	certPEM, err := ioutil.ReadFile(certFile)
	if err != nil {
		return nil, errors.NewWithCause(errors.TLSError, "failed to read certificate file", err)
	}

	cert, err := tls.X509KeyPair(certPEM, nil)
	if err != nil {
		return nil, errors.NewWithCause(errors.TLSError, "failed to parse certificate", err)
	}

	if len(cert.Certificate) == 0 {
		return nil, errors.New(errors.TLSError, "no certificate found")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, errors.NewWithCause(errors.TLSError, "failed to parse X509 certificate", err)
	}

	return x509Cert, nil
}