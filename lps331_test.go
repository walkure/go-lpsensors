package lpsensors_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/walkure/go-lpsensors"
	"periph.io/x/conn/v3/i2c/i2ctest"
	"periph.io/x/conn/v3/physic"
)

const LPS331A_addr = 0x5c
const LPS331A_CTRL_REG1 = 0x20
const LPS331A_CTRL_REG2 = 0x21
const LPS331A_RES_CONF = 0x10

func init_LPS331AOps() []i2ctest.IO {
	return []i2ctest.IO{
		// Chip ID detection.
		{Addr: LPS331A_addr,
			W: []byte{0x0f},
			R: []byte{0xbb}, //LPS331A
		},
		// CTRL_REG1 show
		{Addr: LPS331A_addr,
			W: []byte{LPS331A_CTRL_REG1},
			R: []byte{0xff},
		},
		// CTRL_REG2 show
		{Addr: LPS331A_addr,
			W: []byte{LPS331A_CTRL_REG2},
			R: []byte{0xff},
		},
		// RES_CONF show
		{Addr: LPS331A_addr,
			W: []byte{LPS331A_RES_CONF},
			R: []byte{0xff},
		},
	}
}

func Test_LPS331A_Continuous_Init(t *testing.T) {

	bus := i2ctest.Playback{
		Ops: append(init_LPS331AOps(), i2ctest.IO{
			// CTRL_REG1 setup for continuous measurement
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_CTRL_REG1, 0xe0},
		}),
	}

	_, err := lpsensors.NewI2C(&bus, 0x5c, nil)
	if err != nil {
		t.Fatalf("lps err: %v", err)
	}

}

func Test_LPS331A_Boot(t *testing.T) {
	bus := i2ctest.Playback{
		Ops: append(init_LPS331AOps(),
			i2ctest.IO{
				// CTRL_REG2 set BOOT flag
				Addr: LPS331A_addr,
				W:    []byte{LPS331A_CTRL_REG2, 0b10000000},
			},
			i2ctest.IO{
				// CTRL_REG2 clear BOOT flag
				Addr: LPS331A_addr,
				R:    []byte{0b00000000},
				W:    []byte{LPS331A_CTRL_REG2},
			},
		),
	}

	d, err := lpsensors.NewI2C(&bus, 0x5c, &lpsensors.Opts{
		// DO NOT SEND init command
		Mode: lpsensors.OneShot,
	})
	if err != nil {
		t.Fatalf("lps err: %v", err)
	}

	if err := d.Boot(context.Background()); err != nil {
		t.Fatalf("boot err: %v", err)
	}
}

func Test_LPS331A_SWReset(t *testing.T) {
	bus := i2ctest.Playback{
		Ops: append(init_LPS331AOps(),
			i2ctest.IO{
				// CTRL_REG2 set SWRESET flag
				Addr: LPS331A_addr,
				W:    []byte{LPS331A_CTRL_REG2, 0b100},
			},
			i2ctest.IO{
				// CTRL_REG2 clear SWRESET flag
				Addr: LPS331A_addr,
				W:    []byte{LPS331A_CTRL_REG2, 0b000},
			},
			i2ctest.IO{
				// discard PRESS and TEMP data to clear STATUS_REG
				Addr: LPS331A_addr,
				W:    []byte{0x28 | 0x80},
				R:    []byte{0x00, 0x00, 0x00, 0x00, 0x00},
			},
		),
	}

	d, err := lpsensors.NewI2C(&bus, 0x5c, &lpsensors.Opts{
		// DO NOT SEND init command
		Mode: lpsensors.OneShot,
	})
	if err != nil {
		t.Fatalf("lps err: %v", err)
	}

	if err := d.SWReset(context.Background()); err != nil {
		t.Fatalf("swreset err: %v", err)
	}
}

func Test_LPS331A_Continuous_Measurement(t *testing.T) {
	ops := append(init_LPS331AOps(),
		i2ctest.IO{
			// CTRL_REG1 setup for continuous measurement
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_CTRL_REG1, 0xe0},
		},
		i2ctest.IO{
			// Read temperature
			Addr: LPS331A_addr,
			W:    []byte{0x2b | 0x80}, // TEMP_OUT_L, TEMP_OUT_H
			R:    []byte{0xd0, 0x6b},  // (0x6bd0 = 27600) / 480 + 42.5 = 100 degC
		},
		i2ctest.IO{
			// Read pressure
			Addr: LPS331A_addr,
			W:    []byte{0x28 | 0x80},      // PRESS_OUT_XL , PRESS_OUT_L, PRESS_OUT_H
			R:    []byte{0x00, 0x50, 0x3f}, // (0x3f5000=4149248) / 4096 = 1013 hPa
		},
	)

	//slog.SetLogLoggerLevel(slog.LevelDebug)
	bus := i2ctest.Playback{
		Ops: ops,
	}

	d, err := lpsensors.NewI2C(&bus, 0x5c, nil)
	if err != nil {
		t.Fatalf("lps err: %v", err)
	}

	data := lpsensors.SensorValues{}
	if err := d.Sense(context.TODO(), &data); err != nil {
		t.Fatalf("sense err: %v", err)
	}

	var tc physic.Temperature
	tc.Set("100C")

	var tp physic.Pressure
	tp.Set("101.3kPa")

	assert.Equal(t, tc, data.Temperature)
	assert.Equal(t, tp, data.Pressure)

}

func Test_LPS331A_OneShot_Measurement(t *testing.T) {

	ops := append(init_LPS331AOps(),
		i2ctest.IO{
			// CTRL_REG1 power-off device
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_CTRL_REG1, 0x00},
		},
		i2ctest.IO{
			// RES_CONF set resolution
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_RES_CONF, 0x7a},
		},
		i2ctest.IO{
			// CTRL_REG1 power-on as one-shot mode and enable BDU feature.
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_CTRL_REG1, 0b10000100},
		},
		i2ctest.IO{
			// CTRL_REG2 set ONE_SHOT flag as up (start measurement)
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_CTRL_REG2, 0x01},
		},
		i2ctest.IO{
			// CTRL_REG2 check ONE_SHOT flag as down (measurement done)
			Addr: LPS331A_addr,
			W:    []byte{LPS331A_CTRL_REG2},
			R:    []byte{0x00},
		},
		i2ctest.IO{
			// Read temperature
			Addr: LPS331A_addr,
			W:    []byte{0x2b | 0x80}, // TEMP_OUT_L, TEMP_OUT_H
			R:    []byte{0xd0, 0x6b},  // (0x6bd0 = 27600) / 480 + 42.5 = 100 degC
		},
		i2ctest.IO{
			// Read pressure
			Addr: LPS331A_addr,
			W:    []byte{0x28 | 0x80},      // PRESS_OUT_XL , PRESS_OUT_L, PRESS_OUT_H
			R:    []byte{0x00, 0x50, 0x3f}, // (0x3f5000=4149248) / 4096 = 1013 hPa
		},
	)

	//slog.SetLogLoggerLevel(slog.LevelDebug)
	bus := i2ctest.Playback{
		Ops: ops,
	}

	d, err := lpsensors.NewI2C(&bus, 0x5c, &lpsensors.Opts{
		Mode: lpsensors.OneShot,
	})
	if err != nil {
		t.Fatalf("lps err: %v", err)
	}

	data := lpsensors.SensorValues{}
	if err := d.Sense(context.TODO(), &data); err != nil {
		t.Fatalf("sense err: %v", err)
	}

	var tc physic.Temperature
	tc.Set("100C")

	var tp physic.Pressure
	tp.Set("101.3kPa")

	assert.Equal(t, tc, data.Temperature)
	assert.Equal(t, tp, data.Pressure)

}
