package lpsensors

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func (d *Dev) readReg(reg uint8, b []byte) error {
	// SPI bus interface
	if d.isSPI {
		// MSB is 0 for write and 1 for read.
		read := make([]byte, len(b)+1)
		write := make([]byte, len(read))
		// Rest of the write buffer is ignored.
		write[0] = reg
		if err := d.d.Tx(write, read); err != nil {
			return fmt.Errorf("sr: %w", err)
		}
		copy(b, read[1:])
		return nil
	}
	if err := d.d.Tx([]byte{reg}, b); err != nil {
		return fmt.Errorf("ir: %w", err)
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
		return fmt.Errorf("%sw: %w", comType, err)
	}
	return nil
}

func (d *Dev) wrap(err error) error {
	return fmt.Errorf("%s: %w", strings.ToLower(d.name), err)
}

func waitCancel(ctx context.Context, t *time.Timer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (d *Dev) setAndCheckCtrlReg2(ctx context.Context, value byte) error {
	if err := d.writeCommands(
		[]byte{
			d.regs.ctrl_reg2,
			value,
		}); err != nil {
		return fmt.Errorf("setAndCheckCtrlReg2: failed to write value 0b%08b(0x%x) command CTRL_REG2(0x%x): %w",
			value, value, d.regs.ctrl_reg2, err)
	}

	b := [1]byte{}

	// BOOT takes 2.2 msec. SWRESET takes  4 μsec (LPS25H)
	const timeout = 5 * time.Millisecond
	timer := time.NewTimer(timeout)

	for {
		if err := d.readReg(d.regs.ctrl_reg2, b[:]); err != nil {
			return fmt.Errorf("setAndCheckCtrlReg2: failed read from CTRL_REG2(0x%x): %w",
				d.regs.ctrl_reg2, err)
		}
		// Wait for clear the set flag
		if b[0]&value == 0 {
			return nil
		}

		timer.Reset(timeout)
		select {
		case <-ctx.Done():
			return fmt.Errorf("setAndCheckCtrlReg2: %w", ctx.Err())
		case <-timer.C:
			// spin..
		}
	}
}
