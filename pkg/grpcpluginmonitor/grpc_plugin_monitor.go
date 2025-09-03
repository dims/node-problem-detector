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
	"time"

	"k8s.io/klog/v2"

	gpmtypes "k8s.io/node-problem-detector/pkg/grpcpluginmonitor/types"
	"k8s.io/node-problem-detector/pkg/grpcpluginmonitor/server"
	"k8s.io/node-problem-detector/pkg/problemdaemon"
	"k8s.io/node-problem-detector/pkg/problemmetrics"
	"k8s.io/node-problem-detector/pkg/types"
	"k8s.io/node-problem-detector/pkg/util"
	"k8s.io/node-problem-detector/pkg/util/tomb"
)

const GRPCPluginMonitorName = "grpc-plugin-monitor"

func init() {
	problemdaemon.Register(
		GRPCPluginMonitorName,
		types.ProblemDaemonHandler{
			CreateProblemDaemonOrDie: NewGRPCPluginMonitorOrDie,
			CmdOptionDescription:     "Set to config file paths for GRPC plugin monitor.",
		})
}

type grpcPluginMonitor struct {
	configPath string
	config     gpmtypes.GRPCPluginConfig
	conditions []types.Condition
	server     *server.ProblemMonitorServer
	statusChan chan *types.Status
	resultChan chan gpmtypes.Result
	tomb       *tomb.Tomb
}

// NewGRPCPluginMonitorOrDie creates a new grpcPluginMonitor, panic if error occurs.
func NewGRPCPluginMonitorOrDie(configPath string) types.Monitor {
	g := &grpcPluginMonitor{
		configPath: configPath,
		tomb:       tomb.NewTomb(),
	}

	// Read configuration file
	f, err := os.ReadFile(configPath)
	if err != nil {
		klog.Fatalf("Failed to read GRPC plugin monitor configuration file %q: %v", configPath, err)
	}

	err = json.Unmarshal(f, &g.config)
	if err != nil {
		klog.Fatalf("Failed to unmarshal GRPC plugin monitor configuration file %q: %v", configPath, err)
	}

	// Apply configurations
	err = g.config.ApplyConfiguration()
	if err != nil {
		klog.Fatalf("Failed to apply configuration for GRPC plugin monitor %q: %v", configPath, err)
	}

	// Validate configurations
	err = g.config.Validate()
	if err != nil {
		klog.Fatalf("Failed to validate GRPC plugin monitor config %+v: %v", g.config, err)
	}

	klog.Infof("Finished parsing GRPC plugin monitor config file %s: %+v", g.configPath, g.config)

	// Create channels
	g.statusChan = make(chan *types.Status, 1000)
	g.resultChan = make(chan gpmtypes.Result, 1000)

	// Create GRPC server
	g.server = server.NewProblemMonitorServer(g.config, g.resultChan)

	// Initialize metrics if enabled
	if *g.config.EnableMetricsReporting {
		g.initializeProblemMetricsOrDie()
	}

	return g
}

// initializeProblemMetricsOrDie creates problem metrics for all default conditions and sets the value to 0,
// panic if error occurs.
func (g *grpcPluginMonitor) initializeProblemMetricsOrDie() {
	for _, condition := range g.config.DefaultConditions {
		err := problemmetrics.GlobalProblemMetricsManager.SetProblemGauge(condition.Type, condition.Reason, false)
		if err != nil {
			klog.Fatalf("Failed to initialize problem gauge metrics for condition %q, reason %q: %v",
				condition.Type, condition.Reason, err)
		}

		err = problemmetrics.GlobalProblemMetricsManager.IncrementProblemCounter(condition.Reason, 0)
		if err != nil {
			klog.Fatalf("Failed to initialize problem counter metrics for %q: %v", condition.Reason, err)
		}
	}
}

func (g *grpcPluginMonitor) Start() (<-chan *types.Status, error) {
	klog.Infof("Starting GRPC plugin monitor %s", g.configPath)

	// Start GRPC server
	if err := g.server.Start(); err != nil {
		return nil, err
	}

	// Start monitor loop
	go g.monitorLoop()

	return g.statusChan, nil
}

func (g *grpcPluginMonitor) Stop() {
	klog.Infof("Stopping GRPC plugin monitor %s", g.configPath)
	g.tomb.Stop()
	g.server.Stop()
}

// monitorLoop is the main loop of grpcPluginMonitor.
func (g *grpcPluginMonitor) monitorLoop() {
	g.initializeConditions()
	
	if *g.config.SkipInitialStatus {
		klog.Infof("Skipping sending initial status. Using default conditions: %+v", g.conditions)
	} else {
		g.sendInitialStatus()
	}

	for {
		select {
		case result, ok := <-g.resultChan:
			if !ok {
				klog.Errorf("Result channel closed: %s", g.configPath)
				return
			}
			klog.V(3).Infof("Received new GRPC result for %s: %+v", g.configPath, result)
			
			// Process each problem in the result
			for _, problem := range result.Problems {
				status := g.generateStatus(result.Source, problem)
				if status != nil {
					klog.V(3).Infof("New status generated: %+v", status)
					g.statusChan <- status
				}
			}

		case <-g.tomb.Stopping():
			g.server.Stop()
			klog.Infof("GRPC plugin monitor stopped: %s", g.configPath)
			g.tomb.Done()
			return
		}
	}
}

// generateStatus generates status from the GRPC result.
func (g *grpcPluginMonitor) generateStatus(source string, problem gpmtypes.Problem) *types.Status {
	timestamp := time.Now()
	var activeProblemEvents []types.Event
	var inactiveProblemEvents []types.Event

	if problem.Type == types.Temp {
		// For temporary problems, only generate event when status indicates a problem
		if problem.Status == gpmtypes.Warning {
			activeProblemEvents = append(activeProblemEvents, types.Event{
				Severity:  problem.Severity,
				Timestamp: timestamp,
				Reason:    problem.Reason,
				Message:   problem.Message,
			})
		}
	} else {
		// For permanent problems that change the condition
		for i := range g.conditions {
			condition := &g.conditions[i]
			if condition.Type == problem.Condition {
				// Find the default condition for this type
				var defaultConditionReason string
				var defaultConditionMessage string
				for j := range g.config.DefaultConditions {
					defaultCondition := &g.config.DefaultConditions[j]
					if defaultCondition.Type == problem.Condition {
						defaultConditionReason = defaultCondition.Reason
						defaultConditionMessage = defaultCondition.Message
						break
					}
				}

				needToUpdateCondition := true
				var newReason string
				var newMessage string
				status := toConditionStatus(problem.Status)
				
				if condition.Status == types.True && status != types.True {
					// Scenario 1: Condition status changes from True to False/Unknown
					newReason = defaultConditionReason
					if status == types.False {
						newMessage = defaultConditionMessage
					} else {
						// When status unknown, the result's message is important for debug
						newMessage = problem.Message
					}
				} else if condition.Status != types.True && status == types.True {
					// Scenario 2: Condition status changes from False/Unknown to True
					newReason = problem.Reason
					newMessage = problem.Message
				} else if condition.Status != status {
					// Scenario 3: Condition status changes from False to Unknown or vice versa
					newReason = defaultConditionReason
					if status == types.False {
						newMessage = defaultConditionMessage
					} else {
						// When status unknown, the result's message is important for debug
						newMessage = problem.Message
					}
				} else if condition.Status == types.True && status == types.True &&
					(condition.Reason != problem.Reason ||
						(*g.config.EnableMessageChangeBasedConditionUpdate && condition.Message != problem.Message)) {
					// Scenario 4: Condition status does not change and it stays true.
					// condition reason changes or
					// condition message changes when message based condition update is enabled.
					newReason = problem.Reason
					newMessage = problem.Message
				} else {
					// Scenario 5: Condition status does not change and it stays False/Unknown.
					needToUpdateCondition = false
				}

				if needToUpdateCondition {
					condition.Transition = timestamp
					condition.Status = status
					condition.Reason = newReason
					condition.Message = newMessage

					updateEvent := util.GenerateConditionChangeEvent(
						condition.Type,
						status,
						newReason,
						newMessage,
						timestamp,
					)

					if status == types.True {
						activeProblemEvents = append(activeProblemEvents, updateEvent)
					} else {
						inactiveProblemEvents = append(inactiveProblemEvents, updateEvent)
					}
				}

				break
			}
		}
	}

	// Update metrics if enabled
	if *g.config.EnableMetricsReporting {
		// Increment problem counter only for active problems which just got detected.
		for _, event := range activeProblemEvents {
			err := problemmetrics.GlobalProblemMetricsManager.IncrementProblemCounter(
				event.Reason, 1)
			if err != nil {
				klog.Errorf("Failed to update problem counter metrics for %q: %v",
					event.Reason, err)
			}
		}
		
		// Update gauge metrics for all conditions
		for _, condition := range g.conditions {
			err := problemmetrics.GlobalProblemMetricsManager.SetProblemGauge(
				condition.Type, condition.Reason, condition.Status == types.True)
			if err != nil {
				klog.Errorf("Failed to update problem gauge metrics for condition %q, reason %q: %v",
					condition.Type, condition.Reason, err)
			}
		}
	}

	// Only create status if there are events or condition changes
	if len(activeProblemEvents) == 0 && len(inactiveProblemEvents) == 0 {
		return nil
	}

	status := &types.Status{
		Source: source,
		Events: append(activeProblemEvents, inactiveProblemEvents...),
		Conditions: g.conditions,
	}

	// Log only if condition has changed
	klog.V(0).Infof("New status generated: %+v", status)
	return status
}

func toConditionStatus(s gpmtypes.Status) types.ConditionStatus {
	switch s {
	case gpmtypes.OK:
		return types.False
	case gpmtypes.Warning:
		return types.True
	default:
		return types.Unknown
	}
}

// sendInitialStatus sends the initial status to the node problem detector.
func (g *grpcPluginMonitor) sendInitialStatus() {
	klog.Infof("Sending initial status for %s with conditions: %+v", g.config.Source, g.conditions)
	// Update the initial status
	g.statusChan <- &types.Status{
		Source:     g.config.Source,
		Conditions: g.conditions,
	}
}

// initializeConditions initializes the internal node conditions.
func (g *grpcPluginMonitor) initializeConditions() {
	g.conditions = initialConditions(g.config.DefaultConditions)
	klog.Infof("Initialized conditions for %s: %+v", g.configPath, g.conditions)
}

func initialConditions(defaults []types.Condition) []types.Condition {
	conditions := make([]types.Condition, len(defaults))
	copy(conditions, defaults)
	for i := range conditions {
		conditions[i].Status = types.False
		conditions[i].Transition = time.Now()
	}
	return conditions
}