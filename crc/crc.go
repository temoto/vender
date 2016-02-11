package crc

const CRC_POLY_93 byte = 0x93

func CRC8_p93(crc, data byte) byte {
	crc ^= data
	var i byte = 0
	for ; i < 8; i++ {
		if (crc & 0x80) != 0 {
			crc <<= 1
			crc ^= CRC_POLY_93
		} else {
			crc <<= 1
		}
	}
	return crc
}

func CRC8_p93_2b(b1, b2 byte) byte {
	out := CRC8_p93(0, b1)
	out = CRC8_p93(out, b2)
	return out
}
