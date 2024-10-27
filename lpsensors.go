package lpsensors

import (
	"context"
	"errors"
	"fmt"
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
		return err
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
		return d.wrap(
			fmt.Errorf("failed to send init command: %w", err))
	}

	return nil
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

// ShowCtrls is a function to show the control registers of the device.
func (d *Dev) ShowCtrls() error {
	b := [1]byte{}
	if err := d.readReg(d.regs.ctrl_reg1, b[:]); err != nil {
		return d.wrap(
			fmt.Errorf("ShowCtrls: failed to read CTRL_REG1(0x%x): %w", d.regs.ctrl_reg1, err))
	}
	reg1 := fmt.Sprintf("%08b(0x%02x)", b[0], b[0])
	//fmt.Printf("CTRL_REG1: %08b(0x%02x)\n", b[0], b[0])

	if err := d.readReg(d.regs.ctrl_reg2, b[:]); err != nil {
		return fmt.Errorf("ShowCtrls: failed to read CTRL_REG2(0x%x): %w", d.regs.ctrl_reg2, err)
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
		return d.wrap(fmt.Errorf("ShowCtrls: failed to read RES_CONF(0x%x): %w", d.regs.res_conf, err))
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
