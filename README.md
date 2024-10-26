# go-lpsensors

The library for some [STMicro's pressure sensors](https://www.st.com/en/mems-and-sensors/pressure-sensors.html).

## usage

```
go get -u github.com/walkure/go-lpsensors
```

This library is based on [periph](https://periph.io/) library.

## supported devices

`0xbb` is the response of "WHO_AM_I" command(`0x0f`).

- Tested
    - [LPS331AP](https://www.st.com/ja/mems-and-sensors/lps331ap.html) (0xbb)
- Hopefully
    - LPS22H (0xb1)
    - LPS25H (0xbd)

## caveats

This library is tested *only* [LPS331AP](https://www.st.com/ja/mems-and-sensors/lps331ap.html) with I2C connection.

## license

Apache 2.0

## author

walkure
