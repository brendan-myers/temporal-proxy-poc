package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"temporal-sa/temporal-cloud-proxy/codec"
	"temporal-sa/temporal-cloud-proxy/crypto"

	"go.temporal.io/sdk/converter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"

	"temporal-sa/temporal-cloud-proxy/auth"
	"temporal-sa/temporal-cloud-proxy/metrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Conn struct {
	mu        sync.RWMutex
	namespace map[string]NamespaceConn
}

type NamespaceConn struct {
	conn        *grpc.ClientConn
	authManager *auth.AuthManager
	authType    string
}

func NewConn() *Conn {
	return &Conn{
		namespace: make(map[string]NamespaceConn),
	}
}

// createKMSClient creates an AWS KMS client
func createKMSClient() *kms.KMS {
	// Use the region from parameter or environment variable
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2" // Default region
	}

	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(region),
	}))

	return kms.New(sess)
}

// AddConnInput contains parameters for adding a new connection
type AddConnInput struct {
	Source              string
	Target              string
	TLSCertPath         string
	TLSKeyPath          string
	EncryptionKeyID     string
	Namespace           string
	AuthManager         *auth.AuthManager
	AuthType            string
	MetricsHandler      metrics.MetricsHandler
	CryptoCachingConfig *crypto.CachingConfig
}

// AddConn adds a new connection to the proxy
func (mc *Conn) AddConn(input AddConnInput) error {
	fmt.Println("Adding connection from", input.Source, "to", input.Target)

	cert, err := tls.LoadX509KeyPair(input.TLSCertPath, input.TLSKeyPath)
	if err != nil {
		return err
	}

	//Initialize AWS KMS client
	kmsClient := createKMSClient()

	codecContext := map[string]string{
		"namespace": input.Namespace,
	}

	clientInterceptor, err := converter.NewPayloadCodecGRPCClientInterceptor(
		converter.PayloadCodecGRPCClientInterceptorOptions{
			Codecs: []converter.PayloadCodec{codec.NewEncryptionCodecWithCaching(
				kmsClient,
				codecContext,
				input.EncryptionKeyID,
				input.MetricsHandler,
				input.CryptoCachingConfig,
			)},
		},
	)
	if err != nil {
		return err
	}

	conn, err := grpc.NewClient(
		input.Target,
		grpc.WithTransportCredentials(credentials.NewTLS(
			&tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		)),
		grpc.WithUnaryInterceptor(clientInterceptor),
	)
	if err != nil {
		return err
	}

	mc.mu.Lock()
	mc.namespace[input.Source] = NamespaceConn{
		conn:        conn,
		authManager: input.AuthManager,
		authType:    input.AuthType,
	}
	mc.mu.Unlock()

	return nil
}

// CloseAll closes all connections
func (mc *Conn) CloseAll() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	var errs []error

	for _, namespace := range mc.namespace {
		if err := namespace.conn.Close(); err != nil {
			errs = append(errs, err)
		}
		if err := namespace.authManager.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Invoke implements the grpc.ClientConnInterface Invoke method
func (mc *Conn) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "unable to read metadata")
	}

	target := md.Get(":authority")

	if len(target) <= 0 {
		return status.Error(codes.InvalidArgument, "metadata missing :authority")
	}
	if len(target) != 1 {
		return status.Error(codes.InvalidArgument, "metadata contains multiple :authority entries")
	}

	// The proxy only listens on one port. If for whatever reason the host contains
	// the port, remove it.
	host, _, err := net.SplitHostPort(target[0])
	if err != nil {
		host = target[0]
	}

	mc.mu.RLock()
	namespace, exists := mc.namespace[host]
	mc.mu.RUnlock()

	if !exists {
		return status.Errorf(codes.InvalidArgument, "invalid target: %s", target[0])
	}

	if namespace.authManager != nil {
		authorization := md.Get("authorization")

		if len(authorization) < 1 {
			return status.Error(codes.InvalidArgument, "metadata is missing authorization")
		} else if len(authorization) > 1 {
			return status.Error(codes.InvalidArgument, "metadata contains multiple authorization entries")
		}

		result, err := namespace.authManager.Authenticate(ctx, namespace.authType, authorization[0])
		if err != nil {
			return status.Errorf(codes.Unknown, "failed to authenticate: %s", err)
		}
		if !result.Authenticated {
			return status.Errorf(codes.Unauthenticated, "invalid token")
		}
	}

	return namespace.conn.Invoke(ctx, method, args, reply, opts...)
}

// NewStream implements the grpc.ClientConnInterface NewStream method
func (mc *Conn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, status.Error(codes.Unimplemented, "streams not supported")
}
