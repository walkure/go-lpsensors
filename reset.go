package lpsensors

import (
	"context"
	"fmt"
	"time"
)

// SWReset is a function to send SWRESET[2] command to the device.
func (d *Dev) SWReset(ctx context.Context) error {

	switch d.chipType {
	case chipLPS331A:
		return d.swResetLPS331(ctx)
	case chipLPS22H, chipLPS25H:
		// set and check SWReset[2]
		if err := d.setAndCheckCtrlReg2(ctx, 0b100); err != nil {
			return d.wrap(fmt.Errorf("SWReset: failed :%w", err))
		}
		return nil
	default:
		return d.wrap(fmt.Errorf("SWReset: unknown device type:%x", d.chipType))
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
		return fmt.Errorf("swResetLPS331: failed to write SWReset command CTRL_REG2(0x%x): %w", d.regs.ctrl_reg2, err)
	}

	// wait for process SWRESET
	timer := time.NewTimer(5 * time.Millisecond)
	if err := waitCancel(ctx, timer); err != nil {
		return fmt.Errorf("swResetLPS331: failed to wait process SWRESET: %w", err)
	}

	// clear CTRL_REG2 (NOT automatically cleared after SWRESET)
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg2,
			0,
		}); err != nil {
		return fmt.Errorf("swResetLPS331: failed to clear SWReset command CTRL_REG2(0x%x): %w", d.regs.ctrl_reg2, err)
	}

	// wait for process...
	timer.Reset(5 * time.Millisecond)
	if err := waitCancel(ctx, timer); err != nil {
		return fmt.Errorf("swResetLPS331: failed to wait clearing SWRESET: %w", err)
	}

	//read PRESS_OUT and TEMP_OUT to clear STATUS_REG
	b := [5]byte{}
	if err := d.readReg(0x28|0x80, b[:5]); err != nil {
		return fmt.Errorf("swResetLPS331: failed to discard STATUS_REG(read PRESS/TEMP_OUT): %w", err)
	}

	return nil

}
