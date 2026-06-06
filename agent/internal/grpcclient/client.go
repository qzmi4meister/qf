package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	DefaultCertFile = "/etc/qf/agent.crt"
	DefaultKeyFile  = "/etc/qf/agent.key"
	DefaultCAFile   = "/etc/qf/ca.crt"
)

// Config holds TLS paths and the CP endpoint.
type Config struct {
	CertFile   string // PEM client cert
	KeyFile    string // PEM client key
	CAFile     string // PEM CA cert for server verification
	CPEndpoint string // host:port
}

// DefaultConfig returns Config with /etc/qf/ defaults.
func DefaultConfig(endpoint string) Config {
	return Config{
		CertFile:   DefaultCertFile,
		KeyFile:    DefaultKeyFile,
		CAFile:     DefaultCAFile,
		CPEndpoint: endpoint,
	}
}

// Client wraps a gRPC mTLS connection to CP.
type Client struct {
	conn *grpc.ClientConn
	svc  qfv1.AgentServiceClient
}

// Dial establishes an mTLS gRPC connection to CP.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	creds, err := loadTLSCredentials(cfg)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: load TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.CPEndpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpcclient: dial %s: %w", cfg.CPEndpoint, err)
	}
	return &Client{conn: conn, svc: qfv1.NewAgentServiceClient(conn)}, nil
}

// Stream opens the bidirectional AgentService.Stream RPC.
func (c *Client) Stream(ctx context.Context) (qfv1.AgentService_StreamClient, error) {
	stream, err := c.svc.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: open stream: %w", err)
	}
	return stream, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

func loadTLSCredentials(cfg Config) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("parse CA cert")
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS13,
	}
	return credentials.NewTLS(tlsCfg), nil
}
