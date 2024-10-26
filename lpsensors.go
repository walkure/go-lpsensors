package lpsensors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"periph.io/x/conn/v3"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
)

const (
	chipLPS331A = 0xbb
	chipLPS25H  = 0xbd
	chipLPS22H  = 0xb1
)

// NewI2C returns a Dev object that communicates over I2C.
func NewI2C(b i2c.Bus, addr uint16, opts *Opts) (*Dev, error) {
	switch addr {
	case 0x5c, 0x5d:
	default:
		return nil, errors.New("lps: given address not supported by device")
	}
	d := &Dev{d: &i2c.Dev{Bus: b, Addr: addr}, isSPI: false}
	if err := d.makeDev(opts); err != nil {
		return nil, err
	}
	return d, nil
}

// NewSPI returns a Dev object that communicates over SPI Mode3.
func NewSPI(p spi.Port, opts *Opts) (*Dev, error) {
	// It works both in Mode0 and Mode3.
	c, err := p.Connect(10*physic.MegaHertz, spi.Mode3, 8)
	if err != nil {
		return nil, fmt.Errorf("lps: %v", err)
	}
	d := &Dev{d: c, isSPI: true}
	if err := d.makeDev(opts); err != nil {
		return nil, err
	}
	return d, nil
}

// MeasurementMode is a mode that measures one time and sleep the device or measures continuously.
type MeasurementMode int

const (
	// OneShot mode is a mode that measures one time and sleep the device.
	OneShot MeasurementMode = iota
	// Continuous mode is a mode that measures continuously (about 10Hz).
	Continuous
)

// Opts is a struct to set the mode of the device.
type Opts struct {
	Mode MeasurementMode
}

// DefaultOpts returns the default options.
func DefaultOpts() *Opts {
	return &Opts{
		Mode: Continuous,
	}
}

// Dev is a handle to the LPS device.
type Dev struct {
	d           conn.Conn
	isSPI       bool
	name        string
	chipType    byte
	oneshotMode bool
	regs        struct {
		ctrl_reg1 byte
		ctrl_reg2 byte
		res_conf  byte
	}
	initCmd byte
}

func (d *Dev) makeDev(opts *Opts) error {

	if opts == nil {
		opts = DefaultOpts()
	}

	var chipType [1]byte
	// Read register 0x0F "Who am I?"
	if err := d.readReg(0x0F, chipType[:]); err != nil {
		return err
	}

	var CTRL_REG1, CTRL_REG2, RES_CONF, ODRs, PD byte

	switch chipType[0] {
	case chipLPS331A:
		d.name = "LPS331A"
		RES_CONF = 0x10
		CTRL_REG1 = 0x20
		CTRL_REG2 = 0x21
		ODRs = 0b110 // Data rate 12.5Hz
		PD = 1
	case chipLPS25H:
		d.name = "LPS25H"
		RES_CONF = 0x10
		CTRL_REG1 = 0x20
		CTRL_REG2 = 0x21
		ODRs = 0b011 // Data rate 12.5Hz
		PD = 1
	case chipLPS22H:
		d.name = "LPS22H"
		RES_CONF = 0x00 // No RES_CONF
		CTRL_REG1 = 0x10
		CTRL_REG2 = 0x11
		ODRs = 0b110 // Data rate 10Hz
		PD = 0       // No PD Flag
	default:
		return fmt.Errorf("lps: unexpected chip Type %x", chipType[0])
	}

	slog.Debug("ChipType",
		"Value", fmt.Sprintf("0x%x", chipType[0]),
		"Name", d.name)
	d.chipType = chipType[0]

	d.regs.ctrl_reg1 = CTRL_REG1
	d.regs.ctrl_reg2 = CTRL_REG2
	d.regs.res_conf = RES_CONF
	d.initCmd = PD<<7 | ODRs<<4

	slog.Debug("Cmds",
		"CTRL_REG1", fmt.Sprintf("0x%02x", CTRL_REG1),
		"CTRL_REG2", fmt.Sprintf("0x%02x", CTRL_REG2),
		"RES_CONF", fmt.Sprintf("0x%02x", RES_CONF),
		"INIT_CMD", fmt.Sprintf("0b%08b(0x%02x)", d.initCmd, d.initCmd),
		"PD", fmt.Sprintf("0b%b", PD),
		"ODRs", fmt.Sprintf("0b%b", ODRs),
	)

	if err := d.ShowCtrls(); err != nil {
		return d.wrap(err)
	}

	return d.Init(opts)
}

// Init initializes the device with options.
func (d *Dev) Init(opts *Opts) error {

	if opts.Mode == OneShot {
		d.oneshotMode = true
		return nil
	}

	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg1,
			d.initCmd,
		}); err != nil {
		return d.wrap(err)
	}

	return nil
}

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

// Sense reads the temperature and pressure from the device.
func (d Dev) Sense(ctx context.Context, e *SensorValues) error {

	if d.oneshotMode {
		if err := d.measureOneshot(ctx); err != nil {
			return d.wrap(err)
		}
	}

	return d.sense(e)
}

// Boot is a function to send BOOT[7] command to the device.
func (d *Dev) Boot(ctx context.Context) error {
	// set and check BOOT[7]
	if err := d.setAndCheckCtrlReg2(ctx, 0b10000000); err != nil {
		return d.wrap(err)
	}

	select {
	case <-ctx.Done():
		return d.wrap(ctx.Err())
	case <-time.After(10 * time.Millisecond):
		return nil
	}
}

// SWReset is a function to send SWRESET[2] command to the device.
func (d *Dev) SWReset(ctx context.Context) error {

	switch d.chipType {
	case chipLPS331A:
		return d.swResetLPS331(ctx)
	case chipLPS22H, chipLPS25H:
		// set and check SWReset[2]
		return d.setAndCheckCtrlReg2(ctx, 0b100)
	default:
		return d.wrap(fmt.Errorf("unknown device type:%x", d.chipType))
	}
}

// ShowCtrls is a function to show the control registers of the device.
func (d *Dev) ShowCtrls() error {
	b := [1]byte{}
	if err := d.readReg(d.regs.ctrl_reg1, b[:]); err != nil {
		return d.wrap(err)
	}
	reg1 := fmt.Sprintf("%08b(0x%02x)", b[0], b[0])
	//fmt.Printf("CTRL_REG1: %08b(0x%02x)\n", b[0], b[0])

	if err := d.readReg(d.regs.ctrl_reg2, b[:]); err != nil {
		return d.wrap(err)
	}
	reg2 := fmt.Sprintf("%08b(0x%02x)", b[0], b[0])
	//fmt.Printf("CTRL_REG2: %08b(0x%02x)\n", b[0], b[0])

	if d.regs.res_conf == 0 {
		slog.Debug("Ctrls", "", slog.GroupValue(
			slog.String(fmt.Sprintf("CTRL_REG1(0x%02x)", d.regs.ctrl_reg1), reg1),
			slog.String(fmt.Sprintf("CTRL_REG2(0x%02x)", d.regs.ctrl_reg2), reg2),
		))
		return nil
	}

	if err := d.readReg(d.regs.res_conf, b[:]); err != nil {
		return d.wrap(err)
	}
	resConf := fmt.Sprintf("%08b(0x%02x)", b[0], b[0])
	//fmt.Printf("RES_CONF : %08b(0x%02x)\n", b[0], b[0])
	slog.Debug("Ctrls", "", slog.GroupValue(
		slog.String(fmt.Sprintf("CTRL_REG1(0x%02x)", d.regs.ctrl_reg1), reg1),
		slog.String(fmt.Sprintf("CTRL_REG2(0x%02x)", d.regs.ctrl_reg2), reg2),
		slog.String(fmt.Sprintf("RES_CONF(0x%02x)", d.regs.res_conf), resConf),
	))

	return nil
}

func waitCancel(ctx context.Context, t *time.Timer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// swResetLPS331 is a function to send SWRESET[2] command to the LPS331 device and wait for the reset.
func (d *Dev) swResetLPS331(ctx context.Context) error {

	const reset = byte(0b100)

	// set SWRESET flag and just a wait
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg2,
			reset,
		}); err != nil {
		return d.wrap(err)
	}

	// wait for process SWRESET
	timer := time.NewTimer(5 * time.Millisecond)
	if err := waitCancel(ctx, timer); err != nil {
		return d.wrap(err)
	}

	// clear CTRL_REG2 (NOT automatically cleared after SWRESET)
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg2,
			0,
		}); err != nil {
		return d.wrap(err)
	}

	// wait for process...
	timer.Reset(5 * time.Millisecond)
	if err := waitCancel(ctx, timer); err != nil {
		return d.wrap(err)
	}

	//read PRESS_OUT and TEMP_OUT to clear STATUS_REG
	b := [5]byte{}
	if err := d.readReg(0x28|0x80, b[:5]); err != nil {
		return d.wrap(err)
	}

	return nil

}

func (d *Dev) setAndCheckCtrlReg2(ctx context.Context, value byte) error {
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg2,
			value,
		}); err != nil {
		return d.wrap(err)
	}

	b := [1]byte{}

	// BOOT takes 2.2 msec. SWRESET takes  4 μsec (LPS25H)
	const timeout = 5 * time.Millisecond
	timer := time.NewTimer(timeout)

	for {
		if err := d.readReg(d.regs.ctrl_reg2, b[:]); err != nil {
			return d.wrap(err)
		}
		// Wait for clear the set flag
		if b[0]&value == 0 {
			return nil
		}

		timer.Reset(timeout)
		select {
		case <-ctx.Done():
			return d.wrap(ctx.Err())
		case <-timer.C:
			// spin..
		}
	}
}

func (d Dev) measureOneshot(ctx context.Context) error {

	// Power down the device (clean start)
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg1,
			0, // turn off
		}); err != nil {
		return d.wrap(err)
	}

	// Set the pressure sensor to higher-precision
	if d.regs.res_conf != 0 {
		var cmd byte
		switch d.chipType {
		case chipLPS25H:
			cmd = 0b00001111 // AVGT1 AVGT0 = 1 (Average 64) AVGP1 AVGP0 = 1 (Average 512)
		case chipLPS331A:
			cmd = 0b01111010 // AVGT2 AVGT1 AVGT0 AVGP3 = 1(Average 512) , AVGT2 AVGT1 AVGT1 = 0 1 0 (Average 4)
		}

		if err := d.writeCommands(
			[]byte{
				d.regs.res_conf, // RES_CONF
				cmd,
			}); err != nil {
			return d.wrap(err)
		}

	}

	// Turn on the pressure sensor analog front end in single shot mode
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg1,
			0b10000100, // PD=1 and BDU=1
		}); err != nil {
		return d.wrap(err)
	}

	// Run one shot measurement (Temperature and Pressure), self clearing bit when done.
	// Wait until the measurement is completed: Wait that reading

	// set and check ONE_SHOT[0]
	return d.setAndCheckCtrlReg2(ctx, 0b1)
}

func (d Dev) sense(e *SensorValues) error {

	// In LPS22 with BDU feature, First read Temp. and then read Pressure.
	// Document said that "To guarantee the correct behavior of BDU feature, PRESS_OUT_H (2Ah) must be the last address read."

	datum := [3]byte{}

	// Read Temperature 0x2b(TEMP_OUT_L) 0x2c(TEMP_OUT_H)
	if err := d.readReg(0x2b|0x80, datum[:2]); err != nil {
		return d.wrap(err)
	}
	//rawTemp := int16(binary.LittleEndian.Uint16(b[3:]))
	rawTemp := int16(datum[1])<<8 | int16(datum[0])

	switch d.chipType {
	case chipLPS331A:
		// = 42.5 + (TEMP_OUT_H & TEMP_OUT_L) / 480
		e.Temperature = physic.ZeroCelsius + 425*physic.Celsius/10 + physic.Temperature(rawTemp)*physic.Celsius/480
	case chipLPS22H:
	case chipLPS25H:
		// 100 [count / degC]
		e.Temperature = physic.ZeroCelsius + physic.Temperature(rawTemp)*physic.Celsius/100
	}

	// Read Pressure 0x28(PRESS_OUT_XL) 0x29(PRESS_OUT_L) 0x2a(PRESS_OUT_H)
	// Read multiple bytes : 0b10000000 = 0x80
	if err := d.readReg(0x28|0x80, datum[:3]); err != nil {
		return d.wrap(err)
	}

	//rawPress := uint64(binary.LittleEndian.Uint32(b[:]))
	rawPress := int32(datum[2])<<16 | int32(datum[1])<<8 | int32(datum[0])

	// rawPress / 4096 -> hPa (10^2 Pa)
	// physic.Pressure = nanoPa (10^−9 Pa)

	// h -> n 10^11: (10^11) / 4096 = (10^11) / 2048 / 2 = 48828125 / 2 = 24414062.5
	const c = (1000 * 1000 * 1000 * 100) / 2048
	e.Pressure = physic.Pressure(uint64(rawPress) * c / 2)

	return nil
}

func (d *Dev) readReg(reg uint8, b []byte) error {
	// SPI bus interface
	if d.isSPI {
		// MSB is 0 for write and 1 for read.
		read := make([]byte, len(b)+1)
		write := make([]byte, len(read))
		// Rest of the write buffer is ignored.
		write[0] = reg
		if err := d.d.Tx(write, read); err != nil {
			return d.wrap(fmt.Errorf("sr: %w", err))
		}
		copy(b, read[1:])
		return nil
	}
	if err := d.d.Tx([]byte{reg}, b); err != nil {
		return d.wrap(fmt.Errorf("ir: %w", err))
	}
	return nil
}

func (d *Dev) writeCommands(b []byte) error {

	comType := "i"
	// SPI bus interface
	if d.isSPI {
		// "SPI write"; set RW(MSB) to 0.
		for i := 0; i < len(b); i += 2 {
			b[i] &^= 0x80
		}
		comType = "s"
	}
	attrs := make([]slog.Attr, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		attrs = append(attrs, slog.String(fmt.Sprintf("0x%02x", b[i]), fmt.Sprintf("<-0x%08b(0x%02x)", b[i+1], b[i+1])))
	}
	slog.Debug("writeCommands", comType, attrs)

	if err := d.d.Tx(b, nil); err != nil {
		return d.wrap(fmt.Errorf("%sw: %w", comType, err))
	}
	return nil
}

func (d *Dev) wrap(err error) error {
	return fmt.Errorf("%s: %w", strings.ToLower(d.name), err)
}
