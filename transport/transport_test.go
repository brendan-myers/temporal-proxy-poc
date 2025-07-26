package transport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"temporal-sa/temporal-cloud-proxy/config"
)

// Mock implementations
type MockConfigProvider struct {
	config config.ProxyConfig
}

func (m *MockConfigProvider) GetProxyConfig() config.ProxyConfig {
	return m.config
}

type MockProxyProvider struct {
	mock.Mock
}

func (m *MockProxyProvider) GetConnectionMux() grpc.ClientConnInterface {
	args := m.Called()
	return args.Get(0).(grpc.ClientConnInterface)
}

func (m *MockProxyProvider) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockProxyProvider) Stop() error {
	args := m.Called()
	return args.Error(0)
}

type MockClientConn struct {
	mock.Mock
}

func (m *MockClientConn) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	mockArgs := m.Called(ctx, method, args, reply, opts)
	return mockArgs.Error(0)
}

func (m *MockClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	mockArgs := m.Called(ctx, desc, method, opts)
	return nil, mockArgs.Error(1)
}

func TestNewTransportProvider_Success(t *testing.T) {
	configProvider := &MockConfigProvider{
		config: config.ProxyConfig{
			Server: config.ServerConfig{
				Port: 7233,
				Host: "0.0.0.0",
			},
			Metrics: config.MetricsConfig{
				Port: 9090,
			},
		},
	}

	logger := zap.NewNop()
	mockProxyProvider := &MockProxyProvider{}
	mockClientConn := &MockClientConn{}

	// Setup mock expectations
	mockProxyProvider.On("GetConnectionMux").Return(mockClientConn)

	// Create a mock lifecycle that doesn't actually do anything
	mockLifecycle := &MockLifecycle{}

	// Test the transport provider creation
	transportProvider, err := newTransportProvider(mockLifecycle, configProvider, logger, mockProxyProvider)

	assert.NoError(t, err)
	assert.NotNil(t, transportProvider)

	// Verify mock expectations
	mockProxyProvider.AssertExpectations(t)
}

func TestTransportProvider_StartStop(t *testing.T) {
	configProvider := &MockConfigProvider{
		config: config.ProxyConfig{
			Server: config.ServerConfig{
				Port: 0, // Use port 0 to let the OS assign a free port
				Host: "127.0.0.1",
			},
		},
	}

	logger := zap.NewNop()
	mockProxyProvider := &MockProxyProvider{}
	mockClientConn := &MockClientConn{}

	// Setup mock expectations
	mockProxyProvider.On("GetConnectionMux").Return(mockClientConn)

	// Create a mock lifecycle
	mockLifecycle := &MockLifecycle{}

	transportProvider, err := newTransportProvider(mockLifecycle, configProvider, logger, mockProxyProvider)
	require.NoError(t, err)

	// Test Start
	err = transportProvider.Start()
	assert.NoError(t, err)

	// Test Stop
	err = transportProvider.Stop()
	assert.NoError(t, err)

	// Verify mock expectations
	mockProxyProvider.AssertExpectations(t)
}

func TestTransportProvider_Configuration(t *testing.T) {
	tests := []struct {
		name   string
		config config.ProxyConfig
	}{
		{
			name: "default configuration",
			config: config.ProxyConfig{
				Server: config.ServerConfig{
					Port: 7233,
					Host: "0.0.0.0",
				},
			},
		},
		{
			name: "custom host and port",
			config: config.ProxyConfig{
				Server: config.ServerConfig{
					Port: 8080,
					Host: "127.0.0.1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configProvider := &MockConfigProvider{config: tt.config}
			logger := zap.NewNop()
			mockProxyProvider := &MockProxyProvider{}
			mockClientConn := &MockClientConn{}

			// Setup mock expectations
			mockProxyProvider.On("GetConnectionMux").Return(mockClientConn)

			// Create a mock lifecycle
			mockLifecycle := &MockLifecycle{}

			transportProvider, err := newTransportProvider(mockLifecycle, configProvider, logger, mockProxyProvider)

			assert.NoError(t, err)
			assert.NotNil(t, transportProvider)

			// Cast to concrete type to verify configuration
			concreteProvider, ok := transportProvider.(*grpcTransportProvider)
			require.True(t, ok)

			assert.Equal(t, tt.config.Server.Host, concreteProvider.host)
			assert.Equal(t, tt.config.Server.Port, concreteProvider.port)

			// Verify mock expectations
			mockProxyProvider.AssertExpectations(t)
		})
	}
}

// Mock lifecycle for testing
type MockLifecycle struct {
	hooks []fx.Hook
}

func (m *MockLifecycle) Append(hook fx.Hook) {
	// In a real test, we might want to capture and execute these hooks
	// For now, we'll just ignore them since we're testing the transport provider directly
	m.hooks = append(m.hooks, hook)
}

// Note: This is a simplified test suite that focuses on the basic functionality
// of the transport provider. In a production environment, you might want to add
// more comprehensive tests including:
// - gRPC server integration tests
// - Error handling scenarios
// - Concurrent request handling
// - Graceful shutdown behavior
//
// However, these would require more complex setup and potentially real network
// connections, which goes beyond the scope of unit testing.
