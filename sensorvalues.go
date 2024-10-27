package lpsensors

import (
	"fmt"
	"log/slog"

	"periph.io/x/conn/v3/physic"
)

// SensorValues is a struct to store the sensor values.
type SensorValues struct {
	Temperature physic.Temperature
	Pressure    physic.Pressure
}

// String satisfies the fmt.Stringer interface.
func (s SensorValues) String() string {
	return fmt.Sprintf("Temperature: %s, Pressure: %s", s.Temperature, s.Pressure)
}

// LogValue satisfies the slog.Value interface.
func (s SensorValues) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("Temperature", s.Temperature.String()),
		slog.String("Pressure", s.Pressure.String()),
	)
}
