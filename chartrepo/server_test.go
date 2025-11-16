package chartrepo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServer_detectHelmVersion(t *testing.T) {
	helmBin := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helmBin = h
	}

	s := &Server{
		HelmBin: helmBin,
	}

	version, err := s.detectHelmVersion()
	require.NoError(t, err, "should detect helm version")
	require.NotNil(t, version, "version should not be nil")

	t.Logf("Detected Helm version: %s", version.String())
	require.True(t, version.Major() == 3 || version.Major() == 4, "should be Helm 3 or 4")
}

func TestServer_IsHelm3(t *testing.T) {
	helmBin := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helmBin = h
	}

	s := &Server{
		HelmBin: helmBin,
	}

	isHelm3 := s.IsHelm3()
	t.Logf("IsHelm3: %v", isHelm3)

	// Should be either true or false, not panic
	require.NotNil(t, s.isHelm3, "isHelm3 should be cached")
}

func TestServer_IsHelm4(t *testing.T) {
	helmBin := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helmBin = h
	}

	s := &Server{
		HelmBin: helmBin,
	}

	isHelm4 := s.IsHelm4()
	t.Logf("IsHelm4: %v", isHelm4)

	// Should be either true or false, not panic
	require.NotNil(t, s.isHelm4, "isHelm4 should be cached")
}

func TestServer_VersionDetectionConsistency(t *testing.T) {
	helmBin := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helmBin = h
	}

	s := &Server{
		HelmBin: helmBin,
	}

	isHelm3 := s.IsHelm3()
	isHelm4 := s.IsHelm4()

	// Should be exactly one of Helm 3 or Helm 4, not both
	require.True(t, isHelm3 != isHelm4, "should be either Helm 3 or Helm 4, not both")
}

func TestServer_Run_Helm3And4(t *testing.T) {
	helmBin := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helmBin = h
	}

	chartsDir := "../testdata/charts"

	s := &Server{
		Port:      0, // Use default port
		ChartsDir: chartsDir,
		HelmBin:   helmBin,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Run(ctx)
	}()

	// Give server time to start
	time.Sleep(1 * time.Second)

	// Cancel context to stop server
	cancel()

	// Wait for server to stop
	select {
	case err := <-errChan:
		// Server should stop gracefully when context is canceled
		if err != nil && err.Error() != "starting server: http: Server closed" {
			t.Logf("Server stopped with: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}

func TestServer_EnvVarOverride(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		envValue string
		expected bool
		checkFn  func(*Server) bool
	}{
		{
			name:     "HELM_X_HELM3 forces Helm 3",
			envVar:   "HELM_X_HELM3",
			envValue: "1",
			expected: true,
			checkFn:  func(s *Server) bool { return s.IsHelm3() },
		},
		{
			name:     "HELM_X_HELM4 forces Helm 4",
			envVar:   "HELM_X_HELM4",
			envValue: "1",
			expected: true,
			checkFn:  func(s *Server) bool { return s.IsHelm4() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env var
			oldVal := os.Getenv(tt.envVar)
			os.Setenv(tt.envVar, tt.envValue)
			defer func() {
				if oldVal == "" {
					os.Unsetenv(tt.envVar)
				} else {
					os.Setenv(tt.envVar, oldVal)
				}
			}()

			s := &Server{
				HelmBin: "helm",
			}

			result := tt.checkFn(s)
			require.Equal(t, tt.expected, result, "env var should override version detection")
		})
	}
}

func TestSplitPackageNameAndVersion(t *testing.T) {
	tests := []struct {
		name            string
		pkg             string
		expectedName    string
		expectedVersion string
	}{
		{
			name:            "simple chart",
			pkg:             "mychart-1.0.0",
			expectedName:    "mychart",
			expectedVersion: "1.0.0",
		},
		{
			name:            "chart with dash in name",
			pkg:             "my-chart-1.0.0",
			expectedName:    "my-chart",
			expectedVersion: "1.0.0",
		},
		{
			name:            "chart with multiple dashes",
			pkg:             "my-awesome-chart-2.3.4",
			expectedName:    "my-awesome-chart",
			expectedVersion: "2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitPackageNameAndVersion(tt.pkg)
			require.Len(t, parts, 2, "should return exactly 2 parts")
			require.Equal(t, tt.expectedName, parts[0], "package name should match")
			require.Equal(t, tt.expectedVersion, parts[1], "version should match")
		})
	}
}
