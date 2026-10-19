package conntrack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"

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
	// version 4: ConnIDs are connection indices, sorted per entry and stored as uvarint deltas.
	encodingVersion = 4
)

type chunkEncoder struct {
	w    io.Writer
	err  error
	buf  []byte
	base uint64
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
	e.base = math.MaxUint64
	for _, bucket := range chunk.Buckets {
		entryCount += len(bucket.Entries)
		for _, entry := range bucket.Entries {
			connIDCount += len(entry.ConnIDs)
			for _, id := range entry.ConnIDs {
				e.base = min(e.base, id)
			}
		}
	}
	if connIDCount == 0 {
		e.base = 0
	}
	e.writeByte(encodingVersion)
	e.writeUVarint(uint64(entryCount))
	e.writeUVarint(uint64(connIDCount))
	e.writeUVarint(e.base)
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
	e.encodeConnState(&chunk.ConnState)
	return e.err
}

func (e *chunkEncoder) encodeConnState(state *ConnState) {
	e.writeUVarint(state.ConnIndex)
	e.writeUVarint(uint64(len(state.Conns)))
	var prev uint64
	for _, conn := range state.Conns {
		e.writeUVarint(conn.Index - prev)
		e.writeUint32(conn.ID)
		prev = conn.Index
	}
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
	prev := e.base
	for _, id := range entry.ConnIDs {
		e.writeUVarint(id - prev)
		prev = id
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
	base       uint64
	entryPool  []BucketEntry
	connIDPool []uint64
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
	d.connIDPool = make([]uint64, d.readUVarint())
	if d.version >= 4 {
		d.base = d.readUVarint()
	}
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
	if d.version >= 4 {
		d.decodeConnState(&chunk.ConnState)
	}
	return d.err
}

func (d *chunkDecoder) decodeConnState(state *ConnState) {
	state.ConnIndex = d.readUVarint()
	n := d.readUVarint()
	if d.err != nil || n == 0 {
		return
	}
	state.Conns = make([]LiveConn, n)
	var index uint64
	for i := range state.Conns {
		index += d.readUVarint()
		state.Conns[i] = LiveConn{ID: d.readUint32(), Index: index}
	}
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
	d.readConnIDs(entry.ConnIDs, entry.DstPort)
	d.connIDPool = d.connIDPool[len(entry.ConnIDs):]
}

func (d *chunkDecoder) readConnIDs(ids []uint64, dstPort uint16) {
	id := d.base
	for i := range ids {
		switch d.version {
		case 1:
			srcPort := d.readUint16()
			ids[i] = uint64(dstPort)<<16 | uint64(srcPort)
		case 2:
			ids[i] = d.readUVarint()
		case 3:
			ids[i] = uint64(d.readUint32())
		default:
			id += d.readUVarint()
			ids[i] = id
		}
	}
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
	if br, ok := r.(byteReader); ok {
		return br
	}
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
