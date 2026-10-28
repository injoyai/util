package crc

import "testing"

func TestCRC8VectorsAndStreaming(t *testing.T) {
	params := []Param8{CRC8, CRC8_CDMA2000, CRC8_DARC, CRC8_DVB_S2, CRC8_EBU, CRC8_I_CODE, CRC8_ITU, CRC8_MAXIM, CRC8_ROHC, CRC8_WCDMA}
	data := []byte("123456789")
	for _, p := range params {
		t.Run(p.Name, func(t *testing.T) {
			table := MakeTable8(p)
			if got := Checksum8(data, table); got != p.Check {
				t.Fatalf("got %x want %x", got, p.Check)
			}
			n := Update8(Init8(table), data[:3], table)
			n = Update8(n, data[3:], table)
			if got := Complete8(n, table); got != p.Check {
				t.Fatalf("incremental got %x", got)
			}
		})
	}
	table := MakeTable16(CRC16_MODBUS)
	n := Update16(Init16(table), data[:3], table)
	n = Update16(n, data[3:], table)
	if got := Complete16(n, table); got != 0x4B37 {
		t.Fatalf("CRC16 incremental %x", got)
	}
}
