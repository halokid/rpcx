package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/smallnest/rpcx/log"
	"io"

	"github.com/smallnest/rpcx/util"
)

var (
	// Compressors are compressors supported by rpcx. You can add customized compressor in Compressors.
	Compressors = map[CompressType]Compressor{
		None: &RawDataCompressor{},
		Gzip: &GzipCompressor{},
	}
)

// MaxMessageLength is the max length of a message.
// Default is 0 that means does not limit length of messages.
// It is used to validate when read messages from io.Reader.
var MaxMessageLength = 0

const (
	magicNumber byte = 0x08
)

func MagicNumber() byte {
	return magicNumber
}

var (
	// ErrMetaKVMissing some keys or values are mssing.
	ErrMetaKVMissing = errors.New("wrong metadata lines. some keys or values are missing")
	// ErrMessageTooLong message is too long
	ErrMessageTooLong = errors.New("message is too long")

	ErrUnsupportedCompressor = errors.New("unsupported compressor")
)

const (
	// ServiceError contains error info of service invocation
	ServiceError = "__rpcx_error__"
)

// MessageType is message type of requests and resposnes.
type MessageType byte

const (
	// Request is message type of request
	Request MessageType = iota
	// Response is message type of response
	Response
)

// MessageStatusType is status of messages.
type MessageStatusType byte

const (
	// Normal is normal requests and responses.
	Normal MessageStatusType = iota
	// Error indicates some errors occur.
	Error
)

// CompressType defines decompression type.
type CompressType byte

const (
	// None does not compress.
	None CompressType = iota
	// Gzip uses gzip compression.
	Gzip
)

// SerializeType defines serialization type of payload.
type SerializeType byte

const (
	// SerializeNone uses raw []byte and don't serialize/deserialize
	SerializeNone SerializeType = iota
	// JSON for payload.
	JSON
	// ProtoBuffer for payload.
	ProtoBuffer
	// MsgPack for payload
	MsgPack
	// Thrift
	// Thrift for payload
	Thrift
)

// Message is the generic type of Request and Response.
type Message struct {
	// Message继承了Header, 可以用Header的方法 
	*Header
	ServicePath   string
	ServiceMethod string
	Metadata      map[string]string
	Payload       []byte
	data          []byte
}

// NewMessage creates an empty message.
func NewMessage() *Message {
	header := Header([12]byte{})
	header[0] = magicNumber

	return &Message{
		Header: &header,
	}
}

// Header is the first part of Message and has fixed size.
// Format:
//
type Header [12]byte

// CheckMagicNumber checks whether header starts rpcx magic number.
// todo: 验证数据header是否以 rpcx 定义的 magicNumber 来做开头，如果不是则为非法数据
func (h Header) CheckMagicNumber() bool {
	return h[0] == magicNumber
}

// Version returns version of rpcx protocol.
func (h Header) Version() byte {
	return h[1]
}

// SetVersion sets version for this header.
func (h *Header) SetVersion(v byte) {
	h[1] = v
}

// MessageType returns the message type.  区别数据究竟是请求还是返回
func (h Header) MessageType() MessageType {
	return MessageType(h[2]&0x80) >> 7
}

// SetMessageType sets message type.
func (h *Header) SetMessageType(mt MessageType) {
	h[2] = h[2] | (byte(mt) << 7)
}

// IsHeartbeat returns whether the message is heartbeat message.
func (h Header) IsHeartbeat() bool {
	// todo: 如果 h[2]原来都是默认的值， 8位全部都是0， 则 &0x40之后， 得值0x40
	return h[2] & 0x40 == 0x40
}

// SetHeartbeat sets the heartbeat flag.
func (h *Header) SetHeartbeat(hb bool) {
	if hb {
		// todo: 如果 h[2]的8位, 值为0x40, 换成二进制是0100 0000
		h[2] = h[2] | 0x40
	} else {
		h[2] = h[2] &^ 0x40					// 即是   h[2] & (^0x40), 结果是重置 h[2] 的8位为0
	}
}

// IsOneway returns whether the message is one-way message.
// If true, server won't send responses.
func (h Header) IsOneway() bool {
	return h[2]&0x20 == 0x20
}

// SetOneway sets the oneway flag.
func (h *Header) SetOneway(oneway bool) {
	if oneway {
		h[2] = h[2] | 0x20
	} else {
		h[2] = h[2] &^ 0x20
	}
}

// CompressType returns compression type of messages.
func (h Header) CompressType() CompressType {
	return CompressType((h[2] & 0x1C) >> 2)
}

// SetCompressType sets the compression type.
func (h *Header) SetCompressType(ct CompressType) {
	h[2] = (h[2] &^ 0x1C) | ((byte(ct) << 2) & 0x1C)
}

// MessageStatusType returns the message status type.
func (h Header) MessageStatusType() MessageStatusType {
	return MessageStatusType(h[2] & 0x03)
}

// SetMessageStatusType sets message status type.
func (h *Header) SetMessageStatusType(mt MessageStatusType) {
	h[2] = (h[2] &^ 0x03) | (byte(mt) & 0x03)
}

// SerializeType returns serialization type of payload.
func (h Header) SerializeType() SerializeType {
	return SerializeType((h[3] & 0xF0) >> 4)
}

// SetSerializeType sets the serialization type.
func (h *Header) SetSerializeType(st SerializeType) {
	h[3] = (h[3] &^ 0xF0) | (byte(st) << 4)
}

// Seq returns sequence number of messages.
func (h Header) Seq() uint64 {
	// 大端序模式的字节转为uint64
	return binary.BigEndian.Uint64(h[4:])
}

// SetSeq sets  sequence number.
func (h *Header) SetSeq(seq uint64) {
	binary.BigEndian.PutUint64(h[4:], seq)
}

// Clone clones from an message.
func (m Message) Clone() *Message {
	header := *m.Header
	c := GetPooledMsg()
	header.SetCompressType(None)
	c.Header = &header
	c.ServicePath = m.ServicePath
	c.ServiceMethod = m.ServiceMethod
	return c
}

// Encode encodes messages. 按照约定的数据格式encode data数据
func (m Message) Encode() []byte {
	meta := encodeMetadata(m.Metadata)

	spL := len(m.ServicePath)
	smL := len(m.ServiceMethod)

	var err error
	payload := m.Payload
	if m.CompressType() != None {
		compressor := Compressors[m.CompressType()]
		if compressor == nil {
			m.SetCompressType(None)
		} else {
			log.ADebug.Print("数据 压缩 方式为----", m.Metadata["Content-Encoding"], m.CompressType())
			payload, err = compressor.Zip(m.Payload)
			if err != nil {
				m.SetCompressType(None)
				payload = m.Payload
			}
		}
	}

	totalL := (4 + spL) + (4 + smL) + (4 + len(meta)) + (4 + len(payload))

	// header + dataLen + spLen + sp + smLen + sm + metaL + meta + payloadLen + payload
	metaStart := 12 + 4 + (4 + spL) + (4 + smL)

	payLoadStart := metaStart + (4 + len(meta))
	l := 12 + 4 + totalL

	data := make([]byte, l)
	copy(data, m.Header[:])

	//totalLen
	binary.BigEndian.PutUint32(data[12:16], uint32(totalL))		// 在data的12到16的位置写入数据的总长度

	binary.BigEndian.PutUint32(data[16:20], uint32(spL))			// 在data的16到20的位置写入服务名称的总长度
	copy(data[20:20+spL], util.StringToSliceByte(m.ServicePath)) // 写入服务的名称

	binary.BigEndian.PutUint32(data[20+spL:24+spL], uint32(smL)) // 写入方法名称的长度
	copy(data[24+spL:metaStart], util.StringToSliceByte(m.ServiceMethod))	// 写入方法的名称

	binary.BigEndian.PutUint32(data[metaStart:metaStart+4], uint32(len(meta)))  // meta类似处理
	copy(data[metaStart+4:], meta)

	// payload类似处理
	binary.BigEndian.PutUint32(data[payLoadStart:payLoadStart+4], uint32(len(payload)))
	copy(data[payLoadStart+4:], payload)

	return data
}

// WriteTo writes message to writers.
func (m Message) WriteTo(w io.Writer) error {
	_, err := w.Write(m.Header[:])
	if err != nil {
		return err
	}

	meta := encodeMetadata(m.Metadata)

	spL := len(m.ServicePath)
	smL := len(m.ServiceMethod)

	payload := m.Payload
	if m.CompressType() != None {
		compressor := Compressors[m.CompressType()]
		if compressor == nil {
			return ErrUnsupportedCompressor
		}
		payload, err = compressor.Zip(m.Payload)
		if err != nil {
			return err
		}
	}

	totalL := (4 + spL) + (4 + smL) + (4 + len(meta)) + (4 + len(payload))
	err = binary.Write(w, binary.BigEndian, uint32(totalL))
	if err != nil {
		return err
	}

	//write servicePath and serviceMethod
	err = binary.Write(w, binary.BigEndian, uint32(len(m.ServicePath)))
	if err != nil {
		return err
	}
	_, err = w.Write(util.StringToSliceByte(m.ServicePath))
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.BigEndian, uint32(len(m.ServiceMethod)))
	if err != nil {
		return err
	}
	_, err = w.Write(util.StringToSliceByte(m.ServiceMethod))
	if err != nil {
		return err
	}

	// write meta
	err = binary.Write(w, binary.BigEndian, uint32(len(meta)))
	if err != nil {
		return err
	}
	_, err = w.Write(meta)
	if err != nil {
		return err
	}

	//write payload
	err = binary.Write(w, binary.BigEndian, uint32(len(payload)))
	if err != nil {
		return err
	}

	_, err = w.Write(payload)
	return err
}

// len,string,len,string,......
func encodeMetadata(m map[string]string) []byte {
	if len(m) == 0 {
		return []byte{}
	}
	var buf bytes.Buffer
	var d = make([]byte, 4)
	// todo:  k长度 + k的值 + v长度 +v的值
	for k, v := range m {
		binary.BigEndian.PutUint32(d, uint32(len(k)))
		buf.Write(d)
		buf.Write(util.StringToSliceByte(k))
		binary.BigEndian.PutUint32(d, uint32(len(v)))
		buf.Write(d)
		buf.Write(util.StringToSliceByte(v))
	}
	return buf.Bytes()
}

func decodeMetadata(l uint32, data []byte) (map[string]string, error) {
	m := make(map[string]string, 10)
	n := uint32(0)
	for n < l {
		// parse one key and value
		// key
		sl := binary.BigEndian.Uint32(data[n : n+4])
		n = n + 4
		if n+sl > l-4 {
			return m, ErrMetaKVMissing
		}
		k := string(data[n : n+sl])
		n = n + sl

		// value
		sl = binary.BigEndian.Uint32(data[n : n+4])
		n = n + 4
		if n+sl > l {
			return m, ErrMetaKVMissing
		}
		v := string(data[n : n+sl])
		n = n + sl
		m[k] = v
	}

	return m, nil
}

// Read reads a message from r.
func Read(r io.Reader) (*Message, error) {
	msg := NewMessage()
	err := msg.Decode(r)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// Decode decodes a message from reader.
func (m *Message) Decode(r io.Reader) error {
	// validate rest length for each step?

	//log.Debugf("res.data 2 ---------------------  %+v", m)
	// parse header
	log.ADebug.Print("io.ReadFull读取前m.Header, 已经定义协议头了 ---------- %+v", m.Header)
	_, err := io.ReadFull(r, m.Header[:1])		// todo: 读取协议头
	if err != nil {
		return err
	}
	if !m.Header.CheckMagicNumber() {
		return fmt.Errorf("wrong magic number: %v", m.Header[0])
	}

	_, err = io.ReadFull(r, m.Header[1:])		// todo: 读取协议header
	if err != nil {
		return err
	}
	log.ADebug.Print("io.ReadFull读取后m.Header ---------- %+v", m.Header)

	//total
	// todo: 读取整个 m.Header 的数据, poolUint32Data 就是创建一个 [4][]byte 的数据， 一共32位
	lenData := poolUint32Data.Get().(*[]byte)
	log.ADebug.Print("io.ReadFull读取前lenData ---------- %+v, %+v", lenData, &lenData)
	_, err = io.ReadFull(r, *lenData)		// todo: 读取服务数据总长度
	if err != nil {
		poolUint32Data.Put(lenData)
		return err
	}
	log.ADebug.Print("io.ReadFull读取后lenData ---------- %+v, %+v", lenData, &lenData)
	l := binary.BigEndian.Uint32(*lenData)		// todo: 读取到数据总长度之后， 用大端位的方式解码出来
	poolUint32Data.Put(lenData)

	if MaxMessageLength > 0 && int(l) > MaxMessageLength {
		return ErrMessageTooLong
	}


	//log.Debugf("res.data 3 ---------------------  %+v", m)

	totalL := int(l)
	if cap(m.data) >= totalL { //reuse data
		m.data = m.data[:totalL]
	} else {
		m.data = make([]byte, totalL)
	}

	//log.Debugf("res.data 4 ---------------------  %+v", m)

	data := m.data
	//log.Debugf("m.data 1 ---------------------- %+v", m.data)
	_, err = io.ReadFull(r, data)
	//log.Debugf("m.data 2 ---------------------- %+v", m.data)
	if err != nil {
		return err
	}

	n := 0
	// parse servicePath
	l = binary.BigEndian.Uint32(data[n:4])		// todo: 根据数据的位置读取 message 的总长度数值
	n = n + 4				// todo: 让 n 推后到 服务名称 开始的位置
	nEnd := n + int(l)
	m.ServicePath = util.SliceByteToString(data[n:nEnd])		// todo: 根据数据位置读取 message 的服务名称
	n = nEnd			// todo: 再次推后 n

	// parse serviceMethod
	l = binary.BigEndian.Uint32(data[n : n+4])
	n = n + 4
	nEnd = n + int(l)
	m.ServiceMethod = util.SliceByteToString(data[n:nEnd])			// todo: 读取方法名
	n = nEnd

	// parse meta
	l = binary.BigEndian.Uint32(data[n : n+4])
	n = n + 4
	nEnd = n + int(l)

	if l > 0 {
		m.Metadata, err = decodeMetadata(l, data[n:nEnd])
		if err != nil {
			return err
		}
	}
	n = nEnd

	// todo: parse payload
	l = binary.BigEndian.Uint32(data[n : n+4])
	_ = l
	n = n + 4
	m.Payload = data[n:]

	if m.CompressType() != None {
		compressor := Compressors[m.CompressType()]
		if compressor == nil {
			return ErrUnsupportedCompressor
		}
		log.ADebug.Print("数据 解压缩 方式为----", m.Metadata["Content-Encoding"], m.CompressType())
		m.Payload, err = compressor.Unzip(m.Payload)
		if err != nil {
			return err
		}
	}

	return err
}

// Reset clean data of this message but keep allocated data
func (m *Message) Reset() {
	resetHeader(m.Header)
	m.Metadata = nil
	m.Payload = []byte{}
	m.data = m.data[:0]
	m.ServicePath = ""
	m.ServiceMethod = ""
}

var zeroHeaderArray Header
var zeroHeader = zeroHeaderArray[1:]

func resetHeader(h *Header) {
	copy(h[1:], zeroHeader)
}
