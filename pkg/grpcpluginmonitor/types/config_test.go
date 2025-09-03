/*
Copyright 2024 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGRPCPluginConfig_ApplyConfiguration(t *testing.T) {
	config := &GRPCPluginConfig{
		Source: "test-source",
	}

	err := config.ApplyConfiguration()
	if err != nil {
		t.Fatalf("ApplyConfiguration failed: %v", err)
	}

	// Check defaults
	if config.SocketPath == nil || *config.SocketPath != defaultSocketPath {
		t.Errorf("Expected default socket path %s, got %v", defaultSocketPath, config.SocketPath)
	}

	if config.EnableMetricsReporting == nil || !*config.EnableMetricsReporting {
		t.Errorf("Expected default metrics reporting to be true")
	}

	if config.SkipInitialStatus == nil || *config.SkipInitialStatus {
		t.Errorf("Expected default skip initial status to be false")
	}

	if config.EnableMessageChangeBasedConditionUpdate == nil || *config.EnableMessageChangeBasedConditionUpdate {
		t.Errorf("Expected default message change based condition update to be false")
	}

	if config.TLS.Enabled == nil || !*config.TLS.Enabled {
		t.Errorf("Expected default TLS enabled to be true")
	}
}

func TestGRPCPluginConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  GRPCPluginConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: GRPCPluginConfig{
				Source:     "test-source",
				SocketPath: stringPtr("/tmp/test.sock"),
				TLS: TLSConfig{
					Enabled: boolPtr(false),
				},
			},
			wantErr: false,
		},
		{
			name: "missing source",
			config: GRPCPluginConfig{
				SocketPath: stringPtr("/tmp/test.sock"),
			},
			wantErr: true,
		},
		{
			name: "empty source",
			config: GRPCPluginConfig{
				Source:     "",
				SocketPath: stringPtr("/tmp/test.sock"),
			},
			wantErr: true,
		},
		{
			name: "missing socket path",
			config: GRPCPluginConfig{
				Source: "test-source",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGRPCPluginConfig_ValidateTLS(t *testing.T) {
	// Create temporary directory for test certificates
	tempDir, err := os.MkdirTemp("", "grpc-plugin-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test certificate files
	certFile := filepath.Join(tempDir, "server.crt")
	keyFile := filepath.Join(tempDir, "server.key")
	caFile := filepath.Join(tempDir, "ca.crt")

	for _, file := range []string{certFile, keyFile, caFile} {
		if err := os.WriteFile(file, []byte("test cert"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}

	tests := []struct {
		name    string
		config  GRPCPluginConfig
		wantErr bool
	}{
		{
			name: "valid TLS config",
			config: GRPCPluginConfig{
				Source:     "test-source",
				SocketPath: stringPtr("/tmp/test.sock"),
				TLS: TLSConfig{
					Enabled:           boolPtr(true),
					CertFile:          &certFile,
					KeyFile:           &keyFile,
					CAFile:            &caFile,
					RequireClientCert: boolPtr(true),
				},
			},
			wantErr: false,
		},
		{
			name: "TLS enabled but missing cert file",
			config: GRPCPluginConfig{
				Source:     "test-source",
				SocketPath: stringPtr("/tmp/test.sock"),
				TLS: TLSConfig{
					Enabled: boolPtr(true),
				},
			},
			wantErr: true,
		},
		{
			name: "TLS enabled but cert file doesn't exist",
			config: GRPCPluginConfig{
				Source:     "test-source",
				SocketPath: stringPtr("/tmp/test.sock"),
				TLS: TLSConfig{
					Enabled:  boolPtr(true),
					CertFile: stringPtr("/nonexistent/cert.crt"),
					KeyFile:  stringPtr("/nonexistent/key.key"),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}