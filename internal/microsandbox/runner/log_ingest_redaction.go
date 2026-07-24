package runner

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const redactedEnvironmentValue = "[REDACTED]"

type redactingJournalBody struct {
	reader      *bufio.Reader
	closer      io.Closer
	maxFieldLen int64
	pending     []byte
	pendingErr  error
}

func newRedactingJournalBody(body io.ReadCloser, maxFieldLen int64) io.ReadCloser {
	return &redactingJournalBody{
		reader:      bufio.NewReader(body),
		closer:      body,
		maxFieldLen: maxFieldLen,
	}
}

func (r *redactingJournalBody) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.pendingErr != nil {
			return 0, r.pendingErr
		}
		r.pending, r.pendingErr = r.readField()
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *redactingJournalBody) Close() error {
	return r.closer.Close()
}

func (r *redactingJournalBody) readField() ([]byte, error) {
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return line, err
	}
	if len(line) == 1 {
		return line, nil
	}

	fieldLine := line[:len(line)-1]
	if equals := bytes.IndexByte(fieldLine, '='); equals >= 0 {
		if string(fieldLine[:equals]) != "MESSAGE" {
			return line, nil
		}
		message := redactEnvironmentAssignment(fieldLine[equals+1:])
		out := make([]byte, 0, len("MESSAGE=")+len(message)+1)
		out = append(out, "MESSAGE="...)
		out = append(out, message...)
		out = append(out, '\n')
		return out, nil
	}

	// Journal export encodes fields containing non-printable data as a field
	// name, an unsigned little-endian length, the raw value, and a newline.
	var encodedLength [8]byte
	if _, err := io.ReadFull(r.reader, encodedLength[:]); err != nil {
		return line, err
	}
	valueLength := binary.LittleEndian.Uint64(encodedLength[:])
	const maxInt64 = int64(1<<63 - 1)
	if valueLength > uint64(maxInt64) || int64(valueLength) > r.maxFieldLen {
		return nil, fmt.Errorf("journal field exceeds maximum request size")
	}
	value := make([]byte, valueLength)
	if _, err := io.ReadFull(r.reader, value); err != nil {
		return nil, err
	}
	terminator, err := r.reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if terminator != '\n' {
		return nil, fmt.Errorf("journal binary field is missing terminator")
	}
	if string(fieldLine) == "MESSAGE" {
		value = redactEnvironmentAssignment(value)
		binary.LittleEndian.PutUint64(encodedLength[:], uint64(len(value)))
	}
	out := make([]byte, 0, len(line)+len(encodedLength)+len(value)+1)
	out = append(out, line...)
	out = append(out, encodedLength[:]...)
	out = append(out, value...)
	out = append(out, '\n')
	return out, nil
}

func redactEnvironmentAssignment(message []byte) []byte {
	keyStart := 0
	for keyStart < len(message) && (message[keyStart] == ' ' || message[keyStart] == '\t') {
		keyStart++
	}
	equals := bytes.IndexByte(message[keyStart:], '=')
	if equals <= 0 {
		return message
	}
	equals += keyStart
	for _, ch := range message[keyStart:equals] {
		if ch != '_' && !isASCIILetterOrDigit(ch) {
			return message
		}
	}
	first := message[keyStart]
	if first != '_' && !isASCIILetter(first) {
		return message
	}
	if equals == len(message)-1 {
		return message
	}
	out := make([]byte, 0, equals+1+len(redactedEnvironmentValue))
	out = append(out, message[:equals+1]...)
	out = append(out, redactedEnvironmentValue...)
	return out
}

func isASCIILetter(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func isASCIILetterOrDigit(ch byte) bool {
	return isASCIILetter(ch) || ch >= '0' && ch <= '9'
}
