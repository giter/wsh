package server

// ZMODEM protocol layer (zmodem.h constants, CRC-16, frame encoding and the
// inbound byte-stream parser). The transfer session logic lives in
// zmodem.go.

// ---- Protocol constants (zmodem.h) ----
const (
	zPAD   = 0x2a // '*'
	zDLE   = 0x18 // Ctrl-X
	zHEX   = 0x42 // 'B'
	zBIN   = 0x41 // 'A'
	zBIN32 = 0x43 // 'C'

	zRQINIT = 0  // request receive init (sent by the sender)
	zRINIT  = 1  // receive init (sent by the receiver)
	zACK    = 3  // acknowledge
	zFILE   = 4  // file name follows
	zNAK    = 6  // last packet garbled
	zABORT  = 7  // abort batch
	zFIN    = 8  // finish session
	zRPOS   = 9  // resume at this position
	zDATA   = 10 // data subpackets follow
	zEOF    = 11 // end of file
	zCOMPL  = 15 // request complete
)

// Data subpacket end markers (sent as ZDLE + marker).
const (
	zCRCE = 'h' // crc next, frame ends, header follows
	zCRCG = 'i' // crc next, frame continues non-stop
	zCRCQ = 'j' // crc next, continue, ZACK expected
	zCRCW = 'k' // crc next, ZACK expected, end of frame
)

// ZRINIT capability flags we advertise as receiver.
const (
	zmCANFDX  = 0x01
	zmCANOVIO = 0x02
)

// zmZCBIN is the ZFILE conversion option for a binary transfer.
const zmZCBIN = 1

// zmMaxBlock caps a data subpacket payload.
const zmMaxBlock = 1024

// zmCRC16 computes the XMODEM CRC-16 (poly 0x1021, init 0).
func zmCRC16(data []byte) uint16 {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

const zmHexDigits = "0123456789abcdef"

func zmHexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

// byteWriter is the minimal sink for protocol frames (the SSH stdin pipe).
type byteWriter interface {
	Write(p []byte) (int, error)
}

// ---- Frame emission ----

// zmWriteHexHeader emits a hex header: ZPAD ZPAD ZDLE ZHEX + hex(type, 4
// param bytes, crc16) + CR LF. bigEndian selects the ZRINIT/ZFILE flag
// ordering; offsets (ZRPOS/ZDATA/ZACK/ZEOF) use little-endian.
func zmWriteHexHeader(w byteWriter, typ byte, p0, p1, p2, p3 uint32, bigEndian bool) error {
	body := []byte{typ, 0, 0, 0, 0}
	params := [4]uint32{p0, p1, p2, p3}
	for i := 0; i < 4; i++ {
		if bigEndian {
			body[1+i] = byte(params[i] >> 24)
		} else {
			body[1+i] = byte(params[i])
		}
	}
	crc := zmCRC16(body)
	out := []byte{zPAD, zPAD, zDLE, zHEX}
	for _, b := range body {
		out = append(out, zmHexDigits[b>>4], zmHexDigits[b&0xf])
	}
	out = append(out, zmHexDigits[crc>>12&0xf], zmHexDigits[crc>>8&0xf])
	out = append(out, zmHexDigits[crc>>4&0xf], zmHexDigits[crc&0xf])
	out = append(out, '\r', '\n')
	_, err := w.Write(out)
	return err
}

// zmWriteData emits a binary data subpacket: payload (control bytes escaped)
// followed by ZDLE + end marker + crc16 (escaped).
func zmWriteData(w byteWriter, data []byte, end byte) error {
	out := make([]byte, 0, len(data)+16)
	escape := func(b byte) {
		if b < 0x20 || b == 0x7f {
			out = append(out, zDLE, b^0x40)
		} else {
			out = append(out, b)
		}
	}
	for _, b := range data {
		escape(b)
	}
	crc := zmCRC16(data)
	out = append(out, zDLE, end)
	escape(byte(crc >> 8))
	escape(byte(crc))
	_, err := w.Write(out)
	return err
}

// ---- Inbound parser ----

// zmParser decodes the remote byte stream into headers (hex or binary) and
// data subpackets. Headers carry a type byte plus four parameter bytes.
type zmParser struct {
	state    int
	typ      byte
	params   [4]byte
	paramIdx int
	hex      []byte
	crc      uint16
	crcN     int
	data     []byte
	dataEnd  byte

	onHeader func(typ byte, params [4]byte)
	onData   func(data []byte, end byte)
	onError  func()
}

const (
	psIdle = iota
	psPad
	psZDLE
	psHexType
	psHexParam
	psHexCRC
	psHexCRLF
	psBinType
	psBinEsc
	psBinParam
	psBinParamEsc
	psBinCRC
	psBinCRCEsc
	psData
	psDataEsc
	psDataCRC
	psDataCRCEsc
)

func newZmParser(onHeader func(byte, [4]byte), onData func([]byte, byte), onError func()) *zmParser {
	return &zmParser{
		onHeader: onHeader,
		onData:   onData,
		onError:  onError,
	}
}

// Feed ingests one byte of the stream.
func (p *zmParser) Feed(b byte) {
	switch p.state {
	case psIdle:
		switch b {
		case zPAD:
			p.state = psPad
		case zDLE:
			p.state = psZDLE
		}
	case psPad:
		switch b {
		case zPAD:
			// keep consuming padding
		case zDLE:
			p.state = psZDLE
		default:
			p.state = psIdle
		}
	case psZDLE:
		switch b {
		case zHEX:
			p.hex = p.hex[:0]
			p.state = psHexType
		case zBIN, zBIN32:
			p.hex = p.hex[:0]
			p.state = psBinType
		default:
			p.state = psIdle
		}
	case psHexType:
		if v, ok := zmHexVal(b); ok {
			p.hex = append(p.hex, v)
			if len(p.hex) == 2 {
				p.typ = p.hex[0]<<4 | p.hex[1]
				p.hex = p.hex[:0]
				p.paramIdx = 0
				p.state = psHexParam
			}
		} else {
			p.state = psIdle
		}
	case psHexParam:
		if v, ok := zmHexVal(b); ok {
			p.hex = append(p.hex, v)
			if len(p.hex) == 2 {
				p.params[p.paramIdx] = p.hex[0]<<4 | p.hex[1]
				p.paramIdx++
				p.hex = p.hex[:0]
				if p.paramIdx == 4 {
					p.state = psHexCRC
				}
			}
		} else {
			p.state = psIdle
		}
	case psHexCRC:
		if v, ok := zmHexVal(b); ok {
			p.hex = append(p.hex, v)
			if len(p.hex) == 4 {
				// Hex CRC is two bytes (four hex digits), e.g. "be50".
				p.crc = uint16(p.hex[0])<<12 | uint16(p.hex[1])<<8 |
					uint16(p.hex[2])<<4 | uint16(p.hex[3])
				p.hex = p.hex[:0]
				p.state = psHexCRLF
			}
		} else {
			p.state = psIdle
		}
	case psHexCRLF:
		if b == '\r' || b == '\n' {
			p.finishHeader()
		} else {
			p.state = psIdle
		}
	case psBinType:
		if b == zDLE {
			p.state = psBinEsc
		} else {
			p.typ = b
			p.paramIdx = 0
			p.state = psBinParam
		}
	case psBinEsc:
		p.typ = b ^ 0x40
		p.paramIdx = 0
		p.state = psBinParam
	case psBinParam:
		if b == zDLE {
			p.state = psBinParamEsc
		} else {
			p.params[p.paramIdx] = b
			p.paramIdx++
			if p.paramIdx == 4 {
				p.crc = 0
				p.crcN = 0
				p.state = psBinCRC
			}
		}
	case psBinParamEsc:
		p.params[p.paramIdx] = b ^ 0x40
		p.paramIdx++
		if p.paramIdx == 4 {
			p.crc = 0
			p.crcN = 0
			p.state = psBinCRC
		}
	case psBinCRC:
		if b == zDLE {
			p.state = psBinCRCEsc
		} else {
			p.crc = p.crc<<8 | uint16(b)
			p.crcN++
			if p.crcN == 2 {
				p.finishHeader()
			}
		}
	case psBinCRCEsc:
		p.crc = p.crc<<8 | uint16(b^0x40)
		p.crcN++
		if p.crcN == 2 {
			p.finishHeader()
		}
	case psData:
		if b == zDLE {
			p.state = psDataEsc
		} else {
			p.data = append(p.data, b)
		}
	case psDataEsc:
		switch {
		case b == zCRCE || b == zCRCG || b == zCRCQ || b == zCRCW:
			p.dataEnd = b
			p.crc = 0
			p.crcN = 0
			p.state = psDataCRC
		case b >= 0x40 && b <= 0x5f:
			p.data = append(p.data, b^0x40)
			p.state = psData
		case b == 'l':
			p.data = append(p.data, 0x7f)
			p.state = psData
		case b == 'm':
			p.data = append(p.data, 0xff)
			p.state = psData
		default:
			p.state = psData
		}
	case psDataCRC:
		if b == zDLE {
			p.state = psDataCRCEsc
		} else {
			p.crc = p.crc<<8 | uint16(b)
			p.crcN++
			if p.crcN == 2 {
				p.finishData()
			}
		}
	case psDataCRCEsc:
		p.crc = p.crc<<8 | uint16(b^0x40)
		p.crcN++
		if p.crcN == 2 {
			p.finishData()
		}
	}
}

func (p *zmParser) finishHeader() {
	body := append([]byte{p.typ}, p.params[:]...)
	if zmCRC16(body) != p.crc {
		if p.onError != nil {
			p.onError()
		}
		p.state = psIdle
		return
	}
	if p.onHeader != nil {
		p.onHeader(p.typ, p.params)
	}
	// After ZFILE/ZDATA headers the payload subpackets follow immediately in
	// binary form (no ZPAD preamble).
	if p.typ == zFILE || p.typ == zDATA {
		p.data = p.data[:0]
		p.state = psData
	} else {
		p.state = psIdle
	}
}

func (p *zmParser) finishData() {
	if zmCRC16(p.data) == p.crc {
		if p.onData != nil {
			p.onData(p.data, p.dataEnd)
		}
	}
	p.data = p.data[:0]
	if p.dataEnd == zCRCE || p.dataEnd == zCRCG {
		p.state = psData // stream continues
	} else {
		p.state = psIdle
	}
}

// findZmodemStart scans buf for the beginning of a ZMODEM frame header and
// returns the index of its first byte, or -1 if absent.
func findZmodemStart(buf []byte) int {
	for i := 0; i < len(buf); i++ {
		if buf[i] != zPAD {
			continue
		}
		j := i
		for j < len(buf) && buf[j] == zPAD {
			j++
		}
		if j+1 < len(buf) && buf[j] == zDLE && (buf[j+1] == zHEX || buf[j+1] == zBIN || buf[j+1] == zBIN32) {
			return i
		}
	}
	return -1
}
