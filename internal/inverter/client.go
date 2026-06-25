package inverter

type ModbusReader interface {
	ReadInputRegisters(address, quantity uint16) ([]byte, error)
}

type Client struct{ r ModbusReader }

func NewClient(r ModbusReader) *Client { return &Client{r: r} }

func (c *Client) Read() (Reading, error) {
	b, err := c.r.ReadInputRegisters(0, blockQty)
	if err != nil {
		return Reading{}, err
	}
	return DecodeReading(bytesToRegs(b))
}
