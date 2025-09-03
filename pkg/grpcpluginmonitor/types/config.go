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
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/node-problem-detector/pkg/types"
)

var (
	defaultSocketPath                        = "/var/run/node-problem-detector/grpc-plugin-monitor.sock"
	defaultCertPath                          = "/etc/node-problem-detector/certs"
	defaultEnableMetricsReporting            = true
	defaultSkipInitialStatus                 = false
	defaultEnableMessageChangeBasedConditionUpdate = false
)

// TLSConfig represents TLS configuration for the GRPC server
type TLSConfig struct {
	// Enable TLS for the GRPC server
	Enabled *bool `json:"enabled,omitempty"`
	// Path to server certificate file
	CertFile *string `json:"cert_file,omitempty"`
	// Path to server private key file
	KeyFile *string `json:"key_file,omitempty"`
	// Path to CA certificate file for client verification
	CAFile *string `json:"ca_file,omitempty"`
	// Require client certificates
	RequireClientCert *bool `json:"require_client_cert,omitempty"`
}

// GRPCPluginConfig is the configuration for GRPC plugin monitor
type GRPCPluginConfig struct {
	// Source is the source name of the GRPC plugin monitor
	Source string `json:"source"`
	
	// SocketPath is the unix socket path where the GRPC server will listen
	SocketPath *string `json:"socket_path,omitempty"`
	
	// TLS configuration for the GRPC server
	TLS TLSConfig `json:"tls,omitempty"`
	
	// DefaultConditions are the default states of all conditions this monitor should handle
	DefaultConditions []types.Condition `json:"conditions"`
	
	// EnableMetricsReporting describes whether to report problems as metrics or not
	EnableMetricsReporting *bool `json:"metrics_reporting,omitempty"`
	
	// SkipInitialStatus prevents the first status update with default conditions
	SkipInitialStatus *bool `json:"skip_initial_status,omitempty"`
	
	// EnableMessageChangeBasedConditionUpdate indicates whether NPD should enable message change based condition update
	EnableMessageChangeBasedConditionUpdate *bool `json:"enable_message_change_based_condition_update,omitempty"`
}

// ApplyConfiguration applies default configurations
func (gpc *GRPCPluginConfig) ApplyConfiguration() error {
	if gpc.SocketPath == nil {
		gpc.SocketPath = &defaultSocketPath
	}

	if gpc.EnableMetricsReporting == nil {
		gpc.EnableMetricsReporting = &defaultEnableMetricsReporting
	}

	if gpc.SkipInitialStatus == nil {
		gpc.SkipInitialStatus = &defaultSkipInitialStatus
	}

	if gpc.EnableMessageChangeBasedConditionUpdate == nil {
		gpc.EnableMessageChangeBasedConditionUpdate = &defaultEnableMessageChangeBasedConditionUpdate
	}

	// Apply TLS defaults
	if gpc.TLS.Enabled == nil {
		enabled := true
		gpc.TLS.Enabled = &enabled
	}

	if *gpc.TLS.Enabled {
		if gpc.TLS.CertFile == nil {
			certFile := filepath.Join(defaultCertPath, "server.crt")
			gpc.TLS.CertFile = &certFile
		}

		if gpc.TLS.KeyFile == nil {
			keyFile := filepath.Join(defaultCertPath, "server.key")
			gpc.TLS.KeyFile = &keyFile
		}

		if gpc.TLS.CAFile == nil {
			caFile := filepath.Join(defaultCertPath, "ca.crt")
			gpc.TLS.CAFile = &caFile
		}

		if gpc.TLS.RequireClientCert == nil {
			requireClientCert := true
			gpc.TLS.RequireClientCert = &requireClientCert
		}
	}

	return nil
}

// Validate verifies whether the settings in GRPCPluginConfig are valid
func (gpc GRPCPluginConfig) Validate() error {
	if gpc.Source == "" {
		return fmt.Errorf("source must be specified")
	}

	if gpc.SocketPath == nil || *gpc.SocketPath == "" {
		return fmt.Errorf("socket_path must be specified")
	}

	// Validate socket directory exists or can be created
	socketDir := filepath.Dir(*gpc.SocketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory %q: %v", socketDir, err)
	}

	// Validate TLS configuration if enabled
	if *gpc.TLS.Enabled {
		if gpc.TLS.CertFile == nil || *gpc.TLS.CertFile == "" {
			return fmt.Errorf("cert_file must be specified when TLS is enabled")
		}

		if gpc.TLS.KeyFile == nil || *gpc.TLS.KeyFile == "" {
			return fmt.Errorf("key_file must be specified when TLS is enabled")
		}

		if _, err := os.Stat(*gpc.TLS.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("cert_file %q does not exist", *gpc.TLS.CertFile)
		}

		if _, err := os.Stat(*gpc.TLS.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("key_file %q does not exist", *gpc.TLS.KeyFile)
		}

		if *gpc.TLS.RequireClientCert {
			if gpc.TLS.CAFile == nil || *gpc.TLS.CAFile == "" {
				return fmt.Errorf("ca_file must be specified when require_client_cert is true")
			}

			if _, err := os.Stat(*gpc.TLS.CAFile); os.IsNotExist(err) {
				return fmt.Errorf("ca_file %q does not exist", *gpc.TLS.CAFile)
			}
		}
	}

	return nil
}