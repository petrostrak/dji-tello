package main

import (
	"context"
	"dji-tello/internal/app/drone/command"
	"gobot.io/x/gobot/platforms/dji/tello"
	"log/slog"
	"os"
)

func main() {
	if err := start(); err != nil {
		os.Exit(1)
	}
}

func start() error {
	drone := tello.NewDriver("8888") // Default Tello UDP port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Enable flight data events
	if err := drone.On(tello.ConnectedEvent, func(data interface{}) {
		// Video stream often enables telemetry
		err := drone.StartVideo()
		if err != nil {
			slog.Error("couldn't start video", "err", err)
			return
		}
	}); err != nil {
		slog.Error("couldn't enable data events", "err", err)
		return err
	}

	tc := command.NewTelloController(ctx, drone)
	tc.Start()

	// Define flight plan
	flightPlan := []func() error{
		func() error { return tc.Drone.TakeOff() },
		func() error { return tc.Drone.Up(50) },        // 50 cm up
		func() error { return tc.Drone.Forward(100) },  // 100 cm forward
		func() error { return tc.Drone.Clockwise(90) }, // 90-degree turn
		func() error { return tc.Drone.Backward(50) },  // 50 cm back
		func() error { return tc.Drone.Land() },
	}

	// Execute
	for _, cmd := range flightPlan {
		if err := tc.Execute(cmd); err != nil {
			slog.Error("Flight aborted", "err", err)
			break
		}
	}

	if err := tc.Wait(); err != nil {
		slog.Error("Flight failed", "err", err)
		return err
	} else {
		slog.Info("Flight completed successfully!")
	}

	return nil
}
