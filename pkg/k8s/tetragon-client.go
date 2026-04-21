package k8s

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/mattcarp12/sovereign-sensor/pkg/event"
)

// StreamTetragonEvents manages a resilient connection to Tetragon and pipes normalized events to the channel.
func StreamTetragonEvents(ctx context.Context, serverAddr string, events chan<- event.SovereignEvent) {
	baseBackoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	currentBackoff := baseBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		slog.Info("Attempting connection to Tetragon", "addr", serverAddr)

		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			slog.Warn("gRPC Dial failed", "err", err, "retry_in", currentBackoff)
			time.Sleep(currentBackoff)
			currentBackoff = min(currentBackoff*2, maxBackoff)
			continue
		}

		client := tetragon.NewFineGuidanceSensorsClient(conn)
		stream, err := client.GetEvents(ctx, &tetragon.GetEventsRequest{})
		if err != nil {
			slog.Warn("Failed to open event stream", "err", err, "retry_in", currentBackoff)
			conn.Close()
			time.Sleep(currentBackoff)
			currentBackoff = min(currentBackoff*2, maxBackoff)
			continue
		}

		slog.Info("Successfully subscribed to Tetragon stream")
		currentBackoff = baseBackoff // Reset backoff on success

		// Read from the stream
		for {
			res, err := stream.Recv()
			if err != nil {
				slog.Warn("Stream disconnected. Initiating reconnect...", "err", err)
				break
			}

			if ev := parseEvent(res); ev != nil {
				events <- *ev
			}
		}

		conn.Close()
		time.Sleep(1 * time.Second) // Prevent tight-looping
	}
}

// parseEvent isolates the ugly protobuf type-casting
func parseEvent(res *tetragon.GetEventsResponse) *event.SovereignEvent {
	kpEvent, ok := res.Event.(*tetragon.GetEventsResponse_ProcessKprobe)
	if !ok {
		return nil
	}

	kp := kpEvent.ProcessKprobe
	if kp.FunctionName != "tcp_connect" && kp.FunctionName != "tcp_v4_connect" {
		return nil
	}

	if kp.Process == nil || kp.Process.Pod == nil {
		return nil
	}

	out := &event.SovereignEvent{
		Timestamp: time.Now().UnixNano(),
		PodName:   kp.Process.Pod.Name,
		Namespace: kp.Process.Pod.Namespace,
		Binary:    kp.Process.Binary,
		DestIP:    "unknown",
	}

	if len(kp.Args) > 0 {
		if sockArg, ok := kp.Args[0].GetArg().(*tetragon.KprobeArgument_SockArg); ok {
			out.DestIP = sockArg.SockArg.Daddr
			out.DestPort = sockArg.SockArg.Dport
		}
	}

	return out
}
