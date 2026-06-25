package inverter

import (
	"context"
	"fmt"
	"time"

	"github.com/grid-x/modbus"
)

// modbusAdapter adapts the context-aware modbus.Client to the context-free
// ModbusReader interface.
type modbusAdapter struct{ c modbus.Client }

func (a modbusAdapter) ReadInputRegisters(address, quantity uint16) ([]byte, error) {
	return a.c.ReadInputRegisters(context.Background(), address, quantity)
}

// Open connects to an inverter over an RTU serial handler (9600 8N1, slave 1)
// and returns a Client plus a closer. Used by both `probe` and `run`.
func Open(dev string, timeout time.Duration) (*Client, func(), error) {
	h := modbus.NewRTUClientHandler(dev)
	h.BaudRate = 9600
	h.DataBits = 8
	h.Parity = "N"
	h.StopBits = 1
	h.SlaveID = 1
	h.Timeout = timeout
	if err := h.Connect(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", dev, err)
	}
	return NewClient(modbusAdapter{modbus.NewClient(h)}), func() { _ = h.Close() }, nil
}
