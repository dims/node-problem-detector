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

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/proto"
	gpmtypes "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/types"
)

func TestProblemMonitorServer_ReportProblems(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "grpc-server-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")
	
	config := gpmtypes.GRPCPluginConfig{
		Source:     "test-server",
		SocketPath: &socketPath,
		TLS: gpmtypes.TLSConfig{
			Enabled: boolPtr(false),
		},
	}

	resultChan := make(chan gpmtypes.Result, 10)
	server := NewProblemMonitorServer(config, resultChan)

	tests := []struct {
		name           string
		request        *pb.ReportProblemsRequest
		expectedError  bool
		expectedResult bool
	}{
		{
			name: "valid request with single problem",
			request: &pb.ReportProblemsRequest{
				Source:        "test-client",
				ClientVersion: "1.0.0",
				Problems: []*pb.ProblemReport{
					{
						Type:      pb.ProblemType_PERMANENT,
						Condition: "TestProblem",
						Reason:    "TestFailed",
						Message:   "test has failed",
						Status:    pb.ProblemStatus_WARNING,
						Severity:  pb.ProblemSeverity_WARN,
						Timestamp: timestamppb.Now(),
						Metadata: map[string]string{
							"test": "value",
						},
					},
				},
			},
			expectedError:  false,
			expectedResult: true,
		},
		{
			name: "valid request with no problems",
			request: &pb.ReportProblemsRequest{
				Source:        "test-client",
				ClientVersion: "1.0.0",
				Problems:      []*pb.ProblemReport{},
			},
			expectedError:  false,
			expectedResult: false,
		},
		{
			name: "invalid request - missing source",
			request: &pb.ReportProblemsRequest{
				ClientVersion: "1.0.0",
				Problems: []*pb.ProblemReport{
					{
						Type:     pb.ProblemType_TEMPORARY,
						Reason:   "TestIssue",
						Message:  "test issue",
						Status:   pb.ProblemStatus_WARNING,
						Severity: pb.ProblemSeverity_WARN,
					},
				},
			},
			expectedError:  true,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := server.ReportProblems(ctx, tt.request)
			
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !resp.Success {
				t.Errorf("Expected success=true, got %v", resp.Success)
			}

			expectedCount := int32(len(tt.request.Problems))
			if resp.ProcessedCount != expectedCount {
				t.Errorf("Expected processed count %d, got %d", expectedCount, resp.ProcessedCount)
			}

			if tt.expectedResult && len(tt.request.Problems) > 0 {
				// Check if result was sent to channel
				select {
				case result := <-resultChan:
					if result.Source != tt.request.Source {
						t.Errorf("Expected source %s, got %s", tt.request.Source, result.Source)
					}
					if len(result.Problems) != len(tt.request.Problems) {
						t.Errorf("Expected %d problems, got %d", len(tt.request.Problems), len(result.Problems))
					}
				case <-time.After(1 * time.Second):
					t.Error("Expected result in channel, got timeout")
				}
			}
		})
	}
}

func TestProblemMonitorServer_Check(t *testing.T) {
	config := gpmtypes.GRPCPluginConfig{
		Source: "test-server",
		TLS: gpmtypes.TLSConfig{
			Enabled: boolPtr(false),
		},
	}

	resultChan := make(chan gpmtypes.Result, 10)
	server := NewProblemMonitorServer(config, resultChan)

	// Test health check when server is not serving
	resp, err := server.Check(context.Background(), &pb.HealthCheckRequest{
		Service: "ProblemMonitorService",
	})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if resp.Status != pb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("Expected NOT_SERVING status, got %v", resp.Status)
	}

	// Mark server as serving
	server.mu.Lock()
	server.serving = true
	server.mu.Unlock()

	// Test health check when server is serving
	resp, err = server.Check(context.Background(), &pb.HealthCheckRequest{
		Service: "ProblemMonitorService",
	})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if resp.Status != pb.HealthCheckResponse_SERVING {
		t.Errorf("Expected SERVING status, got %v", resp.Status)
	}
}

func boolPtr(b bool) *bool {
	return &b
}