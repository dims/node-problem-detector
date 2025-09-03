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

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/proto"
)

var (
	socketPath       = flag.String("socket", "/var/run/node-problem-detector/grpc-plugin-monitor.sock", "Path to the unix socket")
	enableTLS        = flag.Bool("tls", true, "Enable TLS")
	certFile         = flag.String("cert", "/etc/node-problem-detector/certs/client.crt", "Client certificate file")
	keyFile          = flag.String("key", "/etc/node-problem-detector/certs/client.key", "Client private key file")
	caFile           = flag.String("ca", "/etc/node-problem-detector/certs/ca.crt", "CA certificate file")
	source           = flag.String("source", "example-client", "Source identifier for this client")
	problemType      = flag.String("type", "permanent", "Problem type (permanent or temporary)")
	condition        = flag.String("condition", "CustomApplicationProblem", "Condition name for permanent problems")
	reason           = flag.String("reason", "ApplicationDown", "Problem reason")
	message          = flag.String("message", "Custom application is not responding", "Problem message")
	status           = flag.String("status", "warning", "Problem status (ok, warning, unknown)")
	severity         = flag.String("severity", "warn", "Problem severity (info, warn, error)")
	healthCheck      = flag.Bool("health", false, "Perform health check instead of reporting problems")
	continousMode    = flag.Bool("continuous", false, "Run in continuous mode, reporting problems every 30 seconds")
	insecureSkipTLS  = flag.Bool("insecure", false, "Skip TLS verification (for testing only)")
)

func main() {
	flag.Parse()

	client, conn, err := createClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer conn.Close()

	if *healthCheck {
		performHealthCheck(client)
		return
	}

	if *continousMode {
		runContinuous(client)
	} else {
		reportSingleProblem(client)
	}
}

func createClient() (pb.ProblemMonitorServiceClient, *grpc.ClientConn, error) {
	var opts []grpc.DialOption

	// Configure dialer for unix socket
	opts = append(opts, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))

	// Configure TLS if enabled
	if *enableTLS {
		if *insecureSkipTLS {
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
		} else {
			tlsConfig, err := createTLSConfig()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create TLS config: %v", err)
			}
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		}
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	// Connect to server
	conn, err := grpc.Dial(*socketPath, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to server: %v", err)
	}

	client := pb.NewProblemMonitorServiceClient(conn)
	return client, conn, nil
}

func createTLSConfig() (*tls.Config, error) {
	// Load client certificate
	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %v", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(*caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %v", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		ServerName:   "localhost", // This should match the server certificate
	}, nil
}

func performHealthCheck(client pb.ProblemMonitorServiceClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Check(ctx, &pb.HealthCheckRequest{
		Service: "ProblemMonitorService",
	})
	if err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Printf("Health check result: %s - %s\n", resp.Status, resp.Message)
}

func reportSingleProblem(client pb.ProblemMonitorServiceClient) {
	problem := createProblem()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.ReportProblems(ctx, &pb.ReportProblemsRequest{
		Source:        *source,
		Problems:      []*pb.ProblemReport{problem},
		ClientVersion: "1.0.0",
	})
	if err != nil {
		log.Fatalf("Failed to report problem: %v", err)
	}

	fmt.Printf("Problem reported successfully: %d problems processed at %s\n", 
		resp.ProcessedCount, resp.ProcessedAt.AsTime().Format(time.RFC3339))
}

func runContinuous(client pb.ProblemMonitorServiceClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	fmt.Println("Running in continuous mode, reporting problems every 30 seconds...")
	
	// Report initial problem
	reportSingleProblem(client)

	for {
		select {
		case <-ticker.C:
			// Alternate between problem and no problem
			problem := createProblem()
			if time.Now().Second()%2 == 0 {
				problem.Status = pb.ProblemStatus_OK
				problem.Message = "Custom application is now healthy"
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			resp, err := client.ReportProblems(ctx, &pb.ReportProblemsRequest{
				Source:        *source,
				Problems:      []*pb.ProblemReport{problem},
				ClientVersion: "1.0.0",
			})
			cancel()

			if err != nil {
				log.Printf("Failed to report problem: %v", err)
			} else {
				fmt.Printf("Problem reported: %d problems processed at %s\n", 
					resp.ProcessedCount, resp.ProcessedAt.AsTime().Format(time.RFC3339))
			}
		}
	}
}

func createProblem() *pb.ProblemReport {
	problem := &pb.ProblemReport{
		Reason:    *reason,
		Message:   *message,
		Timestamp: timestamppb.Now(),
		Metadata: map[string]string{
			"client":    "example-grpc-client",
			"version":   "1.0.0",
			"node_name": getNodeName(),
		},
	}

	// Set problem type
	switch *problemType {
	case "permanent":
		problem.Type = pb.ProblemType_PERMANENT
		problem.Condition = *condition
	case "temporary":
		problem.Type = pb.ProblemType_TEMPORARY
	default:
		log.Fatalf("Invalid problem type: %s (must be 'permanent' or 'temporary')", *problemType)
	}

	// Set problem status
	switch *status {
	case "ok":
		problem.Status = pb.ProblemStatus_OK
	case "warning":
		problem.Status = pb.ProblemStatus_WARNING
	case "unknown":
		problem.Status = pb.ProblemStatus_UNKNOWN
	default:
		log.Fatalf("Invalid status: %s (must be 'ok', 'warning', or 'unknown')", *status)
	}

	// Set severity
	switch *severity {
	case "info":
		problem.Severity = pb.ProblemSeverity_INFO
	case "warn":
		problem.Severity = pb.ProblemSeverity_WARN
	case "error":
		problem.Severity = pb.ProblemSeverity_ERROR
	default:
		log.Fatalf("Invalid severity: %s (must be 'info', 'warn', or 'error')", *severity)
	}

	return problem
}

func getNodeName() string {
	if nodeName := os.Getenv("NODE_NAME"); nodeName != "" {
		return nodeName
	}
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown"
}