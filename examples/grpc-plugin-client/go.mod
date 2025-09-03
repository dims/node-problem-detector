module k8s.io/node-problem-detector/examples/grpc-plugin-client

go 1.24.6

require (
	google.golang.org/grpc v1.74.2
	google.golang.org/protobuf v1.36.6
	k8s.io/node-problem-detector v0.0.0
)

replace k8s.io/node-problem-detector => ../..

require (
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250728155136-f173205681a0 // indirect
)
