package inverter

import (
	"errors"
	"testing"
)

type fakeReader struct {
	data []byte
	err  error
	gotA uint16
	gotQ uint16
}

func (f *fakeReader) ReadInputRegisters(a, q uint16) ([]byte, error) {
	f.gotA, f.gotQ = a, q
	return f.data, f.err
}

func TestClientRead_OK(t *testing.T) {
	regs := make([]uint16, blockQty)
	regs[18] = 80 // SoC
	b := make([]byte, blockQty*2)
	for i, v := range regs {
		b[2*i] = byte(v >> 8)
		b[2*i+1] = byte(v & 0xFF)
	}
	f := &fakeReader{data: b}
	r, err := NewClient(f).Read()
	if err != nil {
		t.Fatal(err)
	}
	if r.SoC != 80 {
		t.Fatalf("SoC: %v", r.SoC)
	}
	if f.gotA != 0 || f.gotQ != blockQty {
		t.Fatalf("read args: addr=%d qty=%d", f.gotA, f.gotQ)
	}
}

func TestClientRead_Error(t *testing.T) {
	f := &fakeReader{err: errors.New("timeout")}
	if _, err := NewClient(f).Read(); err == nil {
		t.Fatal("expected error")
	}
}
