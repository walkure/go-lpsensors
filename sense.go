package lpsensors

import (
	"context"
	"fmt"

	"periph.io/x/conn/v3/physic"
)

// Sense reads the temperature and pressure from the device.
func (d Dev) Sense(ctx context.Context, e *SensorValues) error {

	if d.oneshotMode {
		if err := d.measureOneshot(ctx); err != nil {
			return d.wrap(err)
		}
	}

	if err := d.sense(e); err != nil {
		return d.wrap(err)
	}
	return nil
}

func (d Dev) measureOneshot(ctx context.Context) error {

	// Power down the device (clean start)
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg1,
			0, // turn off
		}); err != nil {
		return fmt.Errorf("measureOneshot: failed to clear CTRL_REG1(0x%x): %w",
			d.regs.ctrl_reg1, err)
	}

	// Set the pressure sensor to higher-precision
	if d.regs.res_conf != 0 {
		var cmd byte
		switch d.chipType {
		case chipLPS25H:
			cmd = 0b00001111 // AVGT1 AVGT0 = 1 (Average 64) AVGP1 AVGP0 = 1 (Average 512)
		case chipLPS331A:
			cmd = 0b01111010 // AVGT2 AVGT1 AVGT0 AVGP3 = 1(Average 512) , AVGT2 AVGT1 AVGT1 = 0 1 0 (Average 4)
		default:
			return fmt.Errorf("measureOneshot: unknown chip type: %v", d.chipType)
		}

		if err := d.writeCommands(
			[]byte{
				d.regs.res_conf, // RES_CONF
				cmd,
			}); err != nil {
			return fmt.Errorf("measureOneshot: failed to write cmd 0b%08b(0x%x) command CTRL_REG2(0x%x): %w",
				cmd, cmd, d.regs.ctrl_reg2, err)
		}

	}

	// Turn on the pressure sensor analog front end in single shot mode
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg1,
			0b10000100, // PD=1 and BDU=1
		}); err != nil {
		return fmt.Errorf("measureOneshot: failed to start ONE_SHOT command to CTRL_REG1(0x%x): %w",
			d.regs.ctrl_reg1, err)
	}

	// Run one shot measurement (Temperature and Pressure), self clearing bit when done.
	// Wait until the measurement is completed: Wait that reading

	// set and check ONE_SHOT[0]
	if err := d.setAndCheckCtrlReg2(ctx, 0b1); err != nil {
		return fmt.Errorf("measureOneshot: failed to set and check ONE_SHOT[0]: %w", err)
	}
	return nil
}

func (d Dev) sense(e *SensorValues) error {

	// In LPS22 with BDU feature, First read Temp. and then read Pressure.
	// Document said that "To guarantee the correct behavior of BDU feature, PRESS_OUT_H (2Ah) must be the last address read."

	datum := [3]byte{}

	// Read Temperature 0x2b(TEMP_OUT_L) 0x2c(TEMP_OUT_H)
	if err := d.readReg(0x2b|0x80, datum[:2]); err != nil {
		return fmt.Errorf("sense: failed to read TEMP_OUT: %w", err)
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
		return fmt.Errorf("sense: failed to read PRESS_OUT: %w", err)
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
