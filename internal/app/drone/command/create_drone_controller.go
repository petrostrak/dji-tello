package command

import (
	"context"
	"fmt"
	"gobot.io/x/gobot/platforms/dji/tello"
	"sync"
	"time"
)

type TelloController struct {
	Drone     *tello.Driver
	commands  chan func() error
	errors    chan error
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	batteryMu sync.Mutex // Protects battery updates
	battery   int8       // Latest battery %
	emergency bool
}

func NewTelloController(ctx context.Context, drone *tello.Driver) *TelloController {
	tc := &TelloController{
		Drone:    drone,
		commands: make(chan func() error, 20),
		errors:   make(chan error, 1),
		ctx:      ctx,
		battery:  -1, // Indicates unknown state
	}
	tc.ctx, tc.cancel = context.WithCancel(ctx)

	// Subscribe to Tello flight data (includes battery)
	drone.On(tello.FlightDataEvent, func(data interface{}) {
		flightData := data.(*tello.FlightData)
		tc.batteryMu.Lock()
		tc.battery = flightData.BatteryPercentage
		tc.batteryMu.Unlock()
	})

	return tc
}

func (tc *TelloController) Start() {
	tc.wg.Add(1)
	go func() {
		defer tc.wg.Done()
		for {
			select {
			case cmd := <-tc.commands:
				if err := cmd(); err != nil {
					tc.errors <- fmt.Errorf("command failed: %w", err)
					tc.emergencyLand() // Auto-trigger emergency procedures
					return
				}
			case <-tc.ctx.Done():
				tc.emergencyLand()
				return
			}
		}
	}()

	// Start battery monitoring
	tc.wg.Add(1)
	go func() {
		defer tc.wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if tc.Battery() < 10 && !tc.emergency {
					tc.errors <- fmt.Errorf("low battery: %d%%", tc.Battery())
					tc.emergencyLand()
					return
				}
			case <-tc.ctx.Done():
				return
			}
		}
	}()
}

func (tc *TelloController) Execute(cmd func() error) error {
	if tc.emergency {
		return fmt.Errorf("emergency state: no new commands accepted")
	}
	select {
	case tc.commands <- cmd:
		return nil
	case <-tc.ctx.Done():
		return tc.ctx.Err()
	}
}

func (tc *TelloController) emergencyLand() {
	tc.emergency = true
	tc.Drone.Land() // Blocking emergency land
	tc.cancel()     // Stop all operations
}

func (tc *TelloController) Wait() error {
	tc.wg.Wait()
	select {
	case err := <-tc.errors:
		return err
	default:
		return nil
	}
}

func (tc *TelloController) Battery() int8 {
	tc.batteryMu.Lock()
	defer tc.batteryMu.Unlock()
	return tc.battery
}
