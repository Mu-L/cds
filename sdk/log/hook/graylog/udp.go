// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.
// inspired from github.com/gemnasium/logrus-graylog-hook

package graylog

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path"
	"time"
)

// Used to control GELF chunking.  Should be less than (MTU - len(UDP
// header)).
//
// TODO: generate dynamically using Path MTU Discovery?
const (
	ChunkSize        = 8154
	chunkedHeaderLen = 12
	chunkedDataLen   = ChunkSize - chunkedHeaderLen
)

var (
	magicChunked = []byte{0x1e, 0x0f}
	magicZlib    = []byte{0x78}
	magicGzip    = []byte{0x1f, 0x8b}
)

// numChunks returns the number of GELF chunks necessary to transmit
// the given compressed buffer.
func numChunks(b []byte) int {
	lenB := len(b)
	if lenB <= ChunkSize {
		return 1
	}
	return int(math.Ceil(float64(lenB) / float64(ChunkSize)))
}

// UDPWriter implements io.Writer and is used to send both discrete
// messages to a graylog2 server, or data from a stream-oriented
// interface (like the functions in log).
type UDPWriter struct {
	conn             net.Conn
	hostname         string
	Facility         string // defaults to current process name
	CompressionLevel int    // one of the consts from compress/flate
	CompressionType  CompressType
}

// NewUDPWriter returns a new GELF Writer.  This writer can be used to send the
// output of the standard Go log functions to a central GELF server by
// passing it to log.SetOutput()
func NewUDPWriter(addr string) (*UDPWriter, error) {
	var err error
	w := new(UDPWriter)
	w.CompressionLevel = flate.BestSpeed

	if w.conn, err = net.Dial("udp", addr); err != nil {
		return nil, err
	}
	if w.hostname, err = os.Hostname(); err != nil {
		return nil, err
	}

	w.Facility = path.Base(os.Args[0])

	return w, nil
}

// writes the gzip compressed byte array to the connection as a series
// of GELF chunked messages.  The header format is documented at
// https://github.com/Graylog2/graylog2-docs/wiki/GELF as:
//
//	2-byte magic (0x1e 0x0f), 8 byte id, 1 byte sequence id, 1 byte
//	total, chunk-data
func (w *UDPWriter) writeChunked(zBytes []byte) (err error) {
	b := make([]byte, 0, ChunkSize)
	buf := bytes.NewBuffer(b)
	nChunksI := numChunks(zBytes)
	if nChunksI > 255 {
		return fmt.Errorf("msg too large, would need %d chunks", nChunksI)
	}
	nChunks := uint8(nChunksI)
	// use urandom to get a unique message id
	msgId := make([]byte, 8)
	rand.Read(msgId)

	bytesLeft := len(zBytes)
	for i := uint8(0); i < nChunks; i++ {
		buf.Reset()
		// manually write header.  Don't care about
		// host/network byte order, because the spec only
		// deals in individual bytes.
		buf.Write(magicChunked) //magic
		buf.Write(msgId)
		buf.WriteByte(i)
		buf.WriteByte(nChunks)
		// slice out our chunk from zBytes
		chunkLen := chunkedDataLen
		if chunkLen > bytesLeft {
			chunkLen = bytesLeft
		}
		off := int(i) * chunkedDataLen
		chunk := zBytes[off : off+chunkLen]
		buf.Write(chunk)

		// write this chunk, and make sure the write was good
		n, err := w.conn.Write(buf.Bytes())
		if err != nil {
			return fmt.Errorf("Write (chunk %d/%d): %s", i,
				nChunks, err)
		}
		if n != len(buf.Bytes()) {
			return fmt.Errorf("Write len: (chunk %d/%d) (%d/%d)",
				i, nChunks, n, len(buf.Bytes()))
		}

		bytesLeft -= chunkLen
	}

	if bytesLeft != 0 {
		return fmt.Errorf("error: %d bytes left after sending", bytesLeft)
	}
	return nil
}

// WriteMessage sends the specified message to the GELF server
// specified in the call to New().  It assumes all the fields are
// filled out appropriately.  In general, clients will want to use
// Write, rather than WriteMessage.
func (w *UDPWriter) WriteMessage(m *Message) (err error) {
	mBytes, err := json.Marshal(m)
	if err != nil {
		return
	}

	var zBuf bytes.Buffer
	var zBytes []byte
	var zw io.WriteCloser
	switch w.CompressionType {
	case CompressGzip:
		zw, err = gzip.NewWriterLevel(&zBuf, w.CompressionLevel)
	case CompressZlib:
		zw, err = zlib.NewWriterLevel(&zBuf, w.CompressionLevel)
	default:
		// no compression
		return errors.New("unknown compression")
	}
	if err != nil {
		return
	}
	if _, err = zw.Write(mBytes); err != nil {
		return
	}
	zw.Close()

	zBytes = zBuf.Bytes()
	if numChunks(zBytes) > 1 {
		return w.writeChunked(zBytes)
	}

	n, err := w.conn.Write(zBytes)
	if err != nil {
		return
	}
	if n != len(zBytes) {
		return fmt.Errorf("bad write (%d/%d)", n, len(zBytes))
	}

	return nil
}

// Write encodes the given string in a GELF message and sends it to
// the server specified in New().
func (w *UDPWriter) Write(p []byte) (n int, err error) {

	// 1 for the function that called us.
	file, line := getCallerIgnoringLogMulti(1)

	// remove trailing and leading whitespace
	p = bytes.TrimSpace(p)

	// If there are newlines in the message, use the first line
	// for the short message and set the full message to the
	// original input.  If the input has no newlines, stick the
	// whole thing in Short.
	short := p
	full := []byte("")
	if i := bytes.IndexRune(p, '\n'); i > 0 {
		short = p[:i]
		full = p
	}

	m := Message{
		Version:  "1.0",
		Host:     w.hostname,
		Short:    string(short),
		Full:     string(full),
		Time:     float64(time.Now().UnixNano()) / 1e9,
		Level:    6, // info
		Facility: w.Facility,
		File:     file,
		Line:     line,
		Extra:    map[string]interface{}{},
	}

	if err = w.WriteMessage(&m); err != nil {
		return 0, err
	}

	return len(p), nil
}
