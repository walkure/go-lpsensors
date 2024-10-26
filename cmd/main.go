package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/walkure/go-lpsensors"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

func main() {

	slog.SetLogLoggerLevel(slog.LevelDebug)

	if _, err := host.Init(); err != nil {
		panic(fmt.Sprint("i2c initialize error: ", err))
	}

	bus, err := i2creg.Open("")
	if err != nil {
		fmt.Println("i2cbus error: ", err)
		return
	}
	defer bus.Close()

	d, err := lpsensors.NewI2C(bus, 0x5c, &lpsensors.Opts{
		//Mode: lpsensors.OneShot,
		Mode: lpsensors.Continuous,
	})
	if err != nil {
		fmt.Println("lps err:", err)
		return
	}
	/*
		if err := d.Boot(context.TODO()); err != nil {
			fmt.Println("boot err:", err)
			return
		}

		if err := d.SWReset(context.TODO()); err != nil {
			fmt.Println("swreset err:", err)
			return
		}

		if err := d.Init(nil); err != nil {
			fmt.Println("init err:", err)
			return
		}
	*/
	data := lpsensors.SensorValues{}
	if err := d.Sense(context.TODO(), &data); err != nil {
		fmt.Println("sense err:", err)
		return
	}

	slog.Info("data", "", data)

}
