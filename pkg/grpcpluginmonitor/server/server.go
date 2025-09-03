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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"

	pb "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/proto"
	gpmtypes "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/types"
	"k8s.io/node-problem-detector/pkg/types"
	"k8s.io/node-problem-detector/pkg/util/tomb"
)

// ProblemMonitorServer implements the GRPC ProblemMonitorService
type ProblemMonitorServer struct {
	pb.UnimplementedProblemMonitorServiceServer
	config     gpmtypes.GRPCPluginConfig
	resultChan chan gpmtypes.Result
	grpcServer *grpc.Server
	tomb       *tomb.Tomb
	mu         sync.RWMutex
	serving    bool
}

// NewProblemMonitorServer creates a new GRPC problem monitor server
func NewProblemMonitorServer(config gpmtypes.GRPCPluginConfig, resultChan chan gpmtypes.Result) *ProblemMonitorServer {
	return &ProblemMonitorServer{
		config:     config,
		resultChan: resultChan,
		tomb:       tomb.NewTomb(),
		serving:    false,
	}
}

// Start starts the GRPC server
func (s *ProblemMonitorServer) Start() error {
	klog.Infof("Starting GRPC plugin monitor server on socket: %s", *s.config.SocketPath)

	// Remove existing socket file if it exists
	if err := os.RemoveAll(*s.config.SocketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %v", err)
	}

	// Create unix socket listener
	listener, err := net.Listen("unix", *s.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %s: %v", *s.config.SocketPath, err)
	}

	// Set socket permissions
	if err := os.Chmod(*s.config.SocketPath, 0660); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %v", err)
	}

	// Create GRPC server with options
	var opts []grpc.ServerOption

	// Configure TLS if enabled
	if *s.config.TLS.Enabled {
		tlsConfig, err := s.createTLSConfig()
		if err != nil {
			listener.Close()
			return fmt.Errorf("failed to create TLS config: %v", err)
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
		klog.Info("GRPC server configured with TLS")
	}

	s.grpcServer = grpc.NewServer(opts...)
	pb.RegisterProblemMonitorServiceServer(s.grpcServer, s)

	s.mu.Lock()
	s.serving = true
	s.mu.Unlock()

	// Start serving in a goroutine
	go func() {
		defer func() {
			s.mu.Lock()
			s.serving = false
			s.mu.Unlock()
			s.tomb.Done()
		}()

		klog.Info("GRPC plugin monitor server started")
		if err := s.grpcServer.Serve(listener); err != nil {
			klog.Errorf("GRPC server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the GRPC server
func (s *ProblemMonitorServer) Stop() {
	klog.Info("Stopping GRPC plugin monitor server")
	s.tomb.Stop()
	
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// Clean up socket file
	if s.config.SocketPath != nil {
		os.RemoveAll(*s.config.SocketPath)
	}
}


// createTLSConfig creates TLS configuration for the server
func (s *ProblemMonitorServer) createTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(*s.config.TLS.CertFile, *s.config.TLS.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	// Configure client certificate verification if required
	if *s.config.TLS.RequireClientCert && s.config.TLS.CAFile != nil {
		caCert, err := os.ReadFile(*s.config.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = caCertPool
		klog.Info("GRPC server configured to require client certificates")
	}

	return tlsConfig, nil
}

// ReportProblems handles incoming problem reports from clients
func (s *ProblemMonitorServer) ReportProblems(ctx context.Context, req *pb.ReportProblemsRequest) (*pb.ReportProblemsResponse, error) {
	klog.V(3).Infof("Received problem report from source: %s with %d problems", req.Source, len(req.Problems))

	if req.Source == "" {
		return nil, status.Errorf(codes.InvalidArgument, "source must be specified")
	}

	if len(req.Problems) == 0 {
		return &pb.ReportProblemsResponse{
			Success:        true,
			ProcessedCount: 0,
			ProcessedAt:    timestamppb.Now(),
		}, nil
	}

	// Convert protobuf problems to internal types
	problems := make([]gpmtypes.Problem, 0, len(req.Problems))
	for _, pbProblem := range req.Problems {
		problem := gpmtypes.Problem{
			Reason:   pbProblem.Reason,
			Message:  pbProblem.Message,
			Metadata: pbProblem.Metadata,
		}

		// Convert problem type
		switch pbProblem.Type {
		case pb.ProblemType_PERMANENT:
			problem.Type = types.Perm
			problem.Condition = pbProblem.Condition
		case pb.ProblemType_TEMPORARY:
			problem.Type = types.Temp
		default:
			klog.Warningf("Unknown problem type: %v, defaulting to temporary", pbProblem.Type)
			problem.Type = types.Temp
		}

		// Convert problem status
		switch pbProblem.Status {
		case pb.ProblemStatus_OK:
			problem.Status = gpmtypes.OK
		case pb.ProblemStatus_WARNING:
			problem.Status = gpmtypes.Warning
		case pb.ProblemStatus_UNKNOWN:
			problem.Status = gpmtypes.Unknown
		default:
			problem.Status = gpmtypes.Unknown
		}

		// Convert severity
		switch pbProblem.Severity {
		case pb.ProblemSeverity_INFO:
			problem.Severity = types.Info
		case pb.ProblemSeverity_WARN:
			problem.Severity = types.Warn
		case pb.ProblemSeverity_ERROR:
			problem.Severity = types.Warn // Map ERROR to WARN since NPD only supports Info/Warn
		default:
			problem.Severity = types.Warn
		}

		problems = append(problems, problem)
	}

	// Create result and send to monitor
	result := gpmtypes.Result{
		Source:    req.Source,
		Problems:  problems,
		Timestamp: time.Now(),
	}

	select {
	case s.resultChan <- result:
		klog.V(3).Infof("Successfully processed %d problems from source: %s", len(problems), req.Source)
	case <-ctx.Done():
		return nil, status.Errorf(codes.DeadlineExceeded, "request timeout")
	case <-s.tomb.Stopping():
		return nil, status.Errorf(codes.Unavailable, "server is shutting down")
	}

	return &pb.ReportProblemsResponse{
		Success:        true,
		ProcessedCount: int32(len(problems)),
		ProcessedAt:    timestamppb.Now(),
	}, nil
}

// Check implements health check service
func (s *ProblemMonitorServer) Check(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.RLock()
	serving := s.serving
	s.mu.RUnlock()

	if serving {
		return &pb.HealthCheckResponse{
			Status:  pb.HealthCheckResponse_SERVING,
			Message: "Server is healthy and serving requests",
		}, nil
	}

	return &pb.HealthCheckResponse{
		Status:  pb.HealthCheckResponse_NOT_SERVING,
		Message: "Server is not serving requests",
	}, nil
}