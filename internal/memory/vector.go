package memory

import (
	"fmt"
	"strconv"
	"strings"
)

func validateVector(vector []float32, dim uint32) error {
	if len(vector) == 0 {
		return fmt.Errorf("embedding vector is empty")
	}
	if dim > 0 && len(vector) != int(dim) {
		return fmt.Errorf("embedding vector dimension %d does not match %d", len(vector), dim)
	}
	return nil
}

func vectorLiteral(vector []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
