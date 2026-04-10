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

const encodingVersion = 1

type chunkEncoder struct {
	w   io.Writer
	err error
	buf []byte
}

func (e *chunkEncoder) Encode(chunk *Chunk) error {
	if e.buf == nil {
		e.buf = make([]byte, 10)
	}
	entryCount := 0
	srcPortCount := 0
	for _, bucket := range chunk.Buckets {
		entryCount += len(bucket.Entries)
		for _, entry := range bucket.Entries {
			srcPortCount += len(entry.SrcPorts)
		}
	}
	e.writeByte(encodingVersion)
	e.writeUint32(uint32(entryCount))
	e.writeUint32(uint32(srcPortCount))
	e.writeTimeRange(chunk.TimeRange)
	e.writeUint16(uint16(chunk.BucketDuration))
	e.writeUint16(uint16(len(chunk.Buckets)))
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
	e.writeUint64(uint64(entry.BytesOrig))
	e.writeUint64(uint64(entry.BytesReply))
	e.writeUint32(entry.PacketsOrig)
	e.writeUint32(entry.PacketsReply)
	e.writeUVarint(uint64(len(entry.SrcPorts)))
	for _, port := range entry.SrcPorts {
		e.writeUint16(port)
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
	e.writeUVarint(uint64(v))
}

func (e *chunkEncoder) writeUint64(v uint64) {
	e.writeUVarint(v)
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
	r           byteReader
	err         error
	buf         []byte
	entryPool   []BucketEntry
	srcPortPool []uint16
}

func (d *chunkDecoder) Decode(chunk *Chunk) error {
	if d.buf == nil {
		d.buf = make([]byte, 10)
	}
	version := d.readByte()
	if version != encodingVersion {
		return errUnexpectedEncodingVersion
	}
	d.entryPool = make([]BucketEntry, d.readUint32())
	d.srcPortPool = make([]uint16, d.readUint32())
	chunk.TimeRange = d.readTimeRange()
	chunk.BucketDuration = uint(d.readUint16())
	chunk.Buckets = make([]Bucket, d.readUint16())
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
	entry.BytesOrig = int64(d.readUint64())
	entry.BytesReply = int64(d.readUint64())
	entry.PacketsOrig = d.readUint32()
	entry.PacketsReply = d.readUint32()
	entry.SrcPorts = d.srcPortPool[:d.readUVarint()]
	for i := range entry.SrcPorts {
		entry.SrcPorts[i] = d.readUint16()
	}
	d.srcPortPool = d.srcPortPool[len(entry.SrcPorts):]
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
	return uint32(d.readUVarint())
}

func (d *chunkDecoder) readUint64() uint64 {
	return d.readUVarint()
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
