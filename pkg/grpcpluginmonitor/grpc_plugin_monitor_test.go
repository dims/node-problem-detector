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

package grpcpluginmonitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	gpmtypes "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/types"
	"k8s.io/node-problem-detector/pkg/types"
)

func TestNewGRPCPluginMonitorOrDie(t *testing.T) {
	// Create temporary directory for socket
	tempDir, err := os.MkdirTemp("", "grpc-monitor-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")
	
	// Create test config file
	config := gpmtypes.GRPCPluginConfig{
		Source:     "test-source",
		SocketPath: &socketPath,
		TLS: gpmtypes.TLSConfig{
			Enabled: boolPtr(false), // Disable TLS for testing
		},
		DefaultConditions: []types.Condition{
			{
				Type:    "TestProblem",
				Reason:  "TestHealthy",
				Message: "test condition",
			},
		},
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	// Create temporary config file
	tempFile, err := os.CreateTemp("", "grpc-plugin-test-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(configBytes); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	tempFile.Close()

	// Test creating monitor
	monitor := NewGRPCPluginMonitorOrDie(tempFile.Name())
	if monitor == nil {
		t.Fatal("Expected monitor to be created, got nil")
	}

	gmonitor, ok := monitor.(*grpcPluginMonitor)
	if !ok {
		t.Fatal("Expected grpcPluginMonitor type")
	}

	if gmonitor.config.Source != "test-source" {
		t.Errorf("Expected source 'test-source', got %s", gmonitor.config.Source)
	}
}

func TestGenerateStatus(t *testing.T) {
	monitor := &grpcPluginMonitor{
		config: gpmtypes.GRPCPluginConfig{
			Source: "test-source",
			DefaultConditions: []types.Condition{
				{
					Type:    "TestProblem",
					Reason:  "TestHealthy",
					Message: "test is healthy",
				},
			},
			EnableMetricsReporting:                      boolPtr(false),
			EnableMessageChangeBasedConditionUpdate: boolPtr(false),
		},
		conditions: []types.Condition{
			{
				Type:       "TestProblem",
				Status:     types.False,
				Reason:     "TestHealthy",
				Message:    "test is healthy",
				Transition: time.Now(),
			},
		},
	}

	tests := []struct {
		name           string
		source         string
		problem        gpmtypes.Problem
		expectedEvents int
	}{
		{
			name:   "temporary problem with warning status",
			source: "test-client",
			problem: gpmtypes.Problem{
				Type:     types.Temp,
				Reason:   "TemporaryIssue",
				Message:  "temporary issue detected",
				Status:   gpmtypes.Warning,
				Severity: types.Warn,
			},
			expectedEvents: 1,
		},
		{
			name:   "temporary problem with ok status",
			source: "test-client",
			problem: gpmtypes.Problem{
				Type:     types.Temp,
				Reason:   "TemporaryIssue",
				Message:  "temporary issue resolved",
				Status:   gpmtypes.OK,
				Severity: types.Info,
			},
			expectedEvents: 0,
		},
		{
			name:   "permanent problem condition change to true",
			source: "test-client",
			problem: gpmtypes.Problem{
				Type:      types.Perm,
				Condition: "TestProblem",
				Reason:    "TestFailed",
				Message:   "test has failed",
				Status:    gpmtypes.Warning,
				Severity:  types.Warn,
			},
			expectedEvents: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := monitor.generateStatus(tt.source, tt.problem)
			
			if tt.expectedEvents == 0 {
				if status != nil {
					t.Errorf("Expected no status to be generated, got %+v", status)
				}
			} else {
				if status == nil {
					t.Fatal("Expected status to be generated, got nil")
				}
				
				if len(status.Events) != tt.expectedEvents {
					t.Errorf("Expected %d events, got %d", tt.expectedEvents, len(status.Events))
				}
				
				if status.Source != tt.source {
					t.Errorf("Expected source %s, got %s", tt.source, status.Source)
				}
			}
		})
	}
}

func TestToConditionStatus(t *testing.T) {
	tests := []struct {
		status   gpmtypes.Status
		expected types.ConditionStatus
	}{
		{gpmtypes.OK, types.False},
		{gpmtypes.Warning, types.True},
		{gpmtypes.Unknown, types.Unknown},
	}

	for _, tt := range tests {
		result := toConditionStatus(tt.status)
		if result != tt.expected {
			t.Errorf("toConditionStatus(%v) = %v, want %v", tt.status, result, tt.expected)
		}
	}
}

func TestInitialConditions(t *testing.T) {
	defaults := []types.Condition{
		{
			Type:    "TestProblem1",
			Reason:  "TestHealthy1",
			Message: "test 1 is healthy",
			Status:  types.True, // This should be reset to False
		},
		{
			Type:    "TestProblem2",
			Reason:  "TestHealthy2",
			Message: "test 2 is healthy",
			Status:  types.Unknown, // This should be reset to False
		},
	}

	conditions := initialConditions(defaults)
	
	if len(conditions) != len(defaults) {
		t.Errorf("Expected %d conditions, got %d", len(defaults), len(conditions))
	}

	for i, condition := range conditions {
		if condition.Status != types.False {
			t.Errorf("Expected condition %d status to be False, got %v", i, condition.Status)
		}
		
		if condition.Type != defaults[i].Type {
			t.Errorf("Expected condition %d type %s, got %s", i, defaults[i].Type, condition.Type)
		}
		
		if condition.Reason != defaults[i].Reason {
			t.Errorf("Expected condition %d reason %s, got %s", i, defaults[i].Reason, condition.Reason)
		}
		
		if condition.Message != defaults[i].Message {
			t.Errorf("Expected condition %d message %s, got %s", i, defaults[i].Message, condition.Message)
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}