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
	"time"

	"k8s.io/node-problem-detector/pkg/types"
)

// Status represents the status of a GRPC plugin check result
type Status int

const (
	OK      Status = 0
	Warning Status = 1
	Unknown Status = 2
)

// Result is the GRPC plugin check result
type Result struct {
	Source    string
	Problems  []Problem
	Timestamp time.Time
}

// Problem represents a single problem report
type Problem struct {
	Type      types.Type
	Condition string
	Reason    string
	Message   string
	Status    Status
	Severity  types.Severity
	Metadata  map[string]string
}