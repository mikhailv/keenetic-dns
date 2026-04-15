package conntrack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

var (
	errUnexpectedIPAddress       = errors.New("unexpected IP address")
	errUnexpectedEncodingVersion = errors.New("unexpected encoding version")
)

const (
	// version 1: SrcPorts []uint16 per entry.
	// version 2: ConnIDs []uvarint per entry.
	// version 3: ConnIDs []uint32 per entry.
	encodingVersion = 3
)

type chunkEncoder struct {
	w   io.Writer
	err error
	buf []byte
}

func newChunkEncoder(w io.Writer) chunkEncoder {
	return chunkEncoder{
		w:   w,
		buf: make([]byte, 10),
	}
}

func (e *chunkEncoder) Encode(chunk *Chunk) error {
	entryCount := 0
	connIDCount := 0
	for _, bucket := range chunk.Buckets {
		entryCount += len(bucket.Entries)
		for _, entry := range bucket.Entries {
			connIDCount += len(entry.ConnIDs)
		}
	}
	e.writeByte(encodingVersion)
	e.writeUVarint(uint64(entryCount))
	e.writeUVarint(uint64(connIDCount))
	e.writeTimeRange(chunk.TimeRange)
	e.writeUVarint(uint64(chunk.BucketDuration))
	e.writeUVarint(uint64(len(chunk.Buckets)))
	for i := range chunk.Buckets {
		buf := e.encodeToBuf(func() {
			e.encodeBucket(&chunk.Buckets[i])
		})
		e.writeUVarint(uint64(len(buf))) // write encoded bucket size for fast skip
		e.write(buf)
	}
	return e.err
}

func (e *chunkEncoder) encodeBucket(bucket *Bucket) {
	e.writeTimeRange(bucket.TimeRange)
	e.writeUVarint(uint64(len(bucket.Entries)))
	for i := range bucket.Entries {
		e.encodeEntry(&bucket.Entries[i])
	}
}

func (e *chunkEncoder) encodeEntry(entry *BucketEntry) {
	e.writeByte(byte(entry.Protocol))
	e.writeIP(entry.SrcIP)
	e.writeIP(entry.DstIP)
	e.writeUint16(entry.DstPort)
	e.writeUVarint(uint64(entry.BytesOrig))
	e.writeUVarint(uint64(entry.BytesReply))
	e.writeUVarint(uint64(entry.PacketsOrig))
	e.writeUVarint(uint64(entry.PacketsReply))
	e.writeUVarint(uint64(len(entry.ConnIDs)))
	for _, id := range entry.ConnIDs {
		e.writeUint32(id)
	}
}

func (e *chunkEncoder) encodeToBuf(encoder func()) []byte {
	if e.err != nil {
		return nil
	}
	var buf bytes.Buffer
	buf.Grow(64 << 10)
	prev := e.w
	e.w = &buf
	encoder()
	e.w = prev
	return buf.Bytes()
}

func (e *chunkEncoder) write(b []byte) {
	if e.err == nil {
		_, e.err = e.w.Write(b)
	}
}

func (e *chunkEncoder) writeByte(b byte) {
	e.buf[0] = b
	e.write(e.buf[:1])
}

func (e *chunkEncoder) writeUint16(v uint16) {
	e.write(binary.LittleEndian.AppendUint16(e.buf[:0], v))
}

func (e *chunkEncoder) writeUint32(v uint32) {
	e.write(binary.LittleEndian.AppendUint32(e.buf[:0], v))
}

func (e *chunkEncoder) writeUint64(v uint64) { //nolint:unused // ignore
	e.write(binary.LittleEndian.AppendUint64(e.buf[:0], v))
}

func (e *chunkEncoder) writeUVarint(v uint64) {
	e.write(binary.AppendUvarint(e.buf[:0], v))
}

func (e *chunkEncoder) writeTimeRange(v TimeRange) {
	e.writeTimestamp(v.Start)
	e.writeTimestamp(v.End)
}

func (e *chunkEncoder) writeTimestamp(v Timestamp) {
	e.write(binary.LittleEndian.AppendUint32(e.buf[:0], uint32(v)))
}

func (e *chunkEncoder) writeIP(ip types.IPv4) {
	if ip.HasPrefix() {
		e.err = errUnexpectedIPAddress
	}
	copy(e.buf, ip[:4])
	e.write(e.buf[:4])
}

type chunkDecoder struct {
	r          byteReader
	err        error
	buf        []byte
	version    byte
	entryPool  []BucketEntry
	connIDPool []uint32
}

func newChunkDecoder(r io.Reader) chunkDecoder {
	return chunkDecoder{
		r:   newByteReader(r),
		buf: make([]byte, 10),
	}
}

func (d *chunkDecoder) Decode(chunk *Chunk, onlyVersion bool) error {
	d.version = d.readByte()
	if d.version == 0 || d.version > encodingVersion {
		return errUnexpectedEncodingVersion
	}
	if onlyVersion {
		return nil
	}
	d.entryPool = make([]BucketEntry, d.readUVarint())
	d.connIDPool = make([]uint32, d.readUVarint())
	chunk.TimeRange = d.readTimeRange()
	if d.version == 1 || d.version == 2 {
		chunk.BucketDuration = uint(d.readUint16())
		chunk.Buckets = make([]Bucket, d.readUint16())
	} else {
		chunk.BucketDuration = uint(d.readUVarint())
		chunk.Buckets = make([]Bucket, d.readUVarint())
	}
	for i := range chunk.Buckets {
		_ = d.readUVarint() // encoded bucket size, ignore for now
		d.decodeBucket(&chunk.Buckets[i])
	}
	return d.err
}

func (d *chunkDecoder) decodeBucket(bucket *Bucket) {
	bucket.TimeRange = d.readTimeRange()
	bucket.Entries = d.entryPool[:d.readUVarint()]
	for i := range bucket.Entries {
		d.decodeEntry(&bucket.Entries[i])
	}
	d.entryPool = d.entryPool[len(bucket.Entries):]
}

func (d *chunkDecoder) decodeEntry(entry *BucketEntry) {
	entry.Protocol = Protocol(d.readByte())
	entry.SrcIP = d.readIP()
	entry.DstIP = d.readIP()
	entry.DstPort = d.readUint16()
	entry.BytesOrig = int64(d.readUVarint())
	entry.BytesReply = int64(d.readUVarint())
	entry.PacketsOrig = uint32(d.readUVarint())
	entry.PacketsReply = uint32(d.readUVarint())
	entry.ConnIDs = d.connIDPool[:d.readUVarint()]
	for i := range entry.ConnIDs {
		switch d.version {
		case 1:
			srcPort := d.readUint16()
			entry.ConnIDs[i] = (uint32(entry.DstPort) << 16) | uint32(srcPort)
		case 2:
			entry.ConnIDs[i] = uint32(d.readUVarint())
		default:
			entry.ConnIDs[i] = d.readUint32()
		}
	}
	d.connIDPool = d.connIDPool[len(entry.ConnIDs):]
}

func (d *chunkDecoder) read(b []byte) {
	if d.err == nil {
		_, d.err = io.ReadFull(d.r, b)
	}
}

func (d *chunkDecoder) readByte() byte {
	b := d.buf[:1]
	d.read(b)
	return b[0]
}

func (d *chunkDecoder) readUint16() uint16 {
	b := d.buf[:2]
	d.read(b)
	return binary.LittleEndian.Uint16(b)
}

func (d *chunkDecoder) readUint32() uint32 {
	b := d.buf[:4]
	d.read(b)
	return binary.LittleEndian.Uint32(b)
}

func (d *chunkDecoder) readUint64() uint64 { //nolint:unused // ignore
	b := d.buf[:8]
	d.read(b)
	return binary.LittleEndian.Uint64(b)
}

func (d *chunkDecoder) readUVarint() uint64 {
	if d.err != nil {
		return 0
	}
	v, err := binary.ReadUvarint(d.r)
	d.err = err
	return v
}

func (d *chunkDecoder) readTimeRange() TimeRange {
	return TimeRange{
		Start: d.readTimestamp(),
		End:   d.readTimestamp(),
	}
}

func (d *chunkDecoder) readTimestamp() Timestamp {
	b := d.buf[:4]
	d.read(b)
	return Timestamp(binary.LittleEndian.Uint32(b))
}

func (d *chunkDecoder) readIP() types.IPv4 {
	var ip types.IPv4
	d.read(d.buf[:4])
	copy(ip[:], d.buf[:4])
	ip[4] = 32
	return ip
}

type byteReader interface {
	io.Reader
	io.ByteReader
}

func newByteReader(r io.Reader) byteReader {
	return &byteReaderImpl{r, make([]byte, 1)}
}

type byteReaderImpl struct {
	io.Reader
	b []byte
}

func (r byteReaderImpl) ReadByte() (byte, error) {
	_, err := io.ReadFull(r.Reader, r.b)
	return r.b[0], err
}
