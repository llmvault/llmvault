package runner

import "fmt"

func toUint16Port(name string, value int) (uint16, error) {
	if value <= 0 || value > 65535 {
		return 0, fmt.Errorf("%s %d is outside tcp port range", name, value)
	}
	return uint16(value), nil
}

func positiveUint8(name string, value int) (uint8, error) {
	if value <= 0 || value > 255 {
		return 0, fmt.Errorf("%s %d is outside uint8 range", name, value)
	}
	return uint8(value), nil
}

func positiveUint32(name string, value int) (uint32, error) {
	if value <= 0 {
		return 0, fmt.Errorf("%s %d must be positive", name, value)
	}
	return uint32(value), nil // #nosec G115 -- value is positive and int cannot exceed uint32 on supported 32/64-bit platforms.
}
