package rtp

import (
	"fmt"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/common/streambuf"
	"github.com/elastic/beats/libbeat/logp"

	"github.com/elastic/beats/packetbeat/protos"
	"github.com/elastic/beats/packetbeat/publish"
)

func printlnf(format string, a ...interface{}) (n int, err error) {
	return fmt.Printf(format+"\n", a...)
}

var (
	debugf = printlnf //logp.MakeDebug("rtp")
)

type rtpPlugin struct {
	ports                     []int
	maxBodyLength             int
	parseHeaders              bool
	parseArguments            bool
	hideConnectionInformation bool

	results publish.Transactions // Channel where results are pushed.
}

type ssrc uint32
type csrc uint32
type rtpTimestamp uint32
type rtpExtendedTimestamp uint64
type rtpSequenceNumber uint16

type rtpHeader struct {
	Version        uint8
	Padded         bool
	WithExtension  bool
	Marked         bool
	PayloadType    uint8
	SequenceNumber rtpSequenceNumber
	Timestamp      rtpTimestamp
	SSRC           ssrc
	CSRC           []csrc
}

type rtpPacket struct {
	Header  rtpHeader
	Payload []byte
}

func (rtp *rtpHeader) Unpack(buf *streambuf.Buffer) *rtpError {
	if !buf.Avail(12) {
		return unexpectedLengthMsg
	}
	byte1, _ := buf.ReadNetUint8()
	rtp.Version = byte1 >> 6
	if rtp.Version != 2 {
		return unknownVersionMsg
	}
	rtp.Padded = (byte1 & 0x20) == 0x20
	rtp.WithExtension = (byte1 & 0x10) == 0x10
	csrcCount := int(byte1 & 0x0F)

	byte2, _ := buf.ReadNetUint8()
	rtp.Marked = (byte2 & 0x80) == 0x80
	rtp.PayloadType = byte2 & 0x7F

	SN, _ := buf.ReadNetUint16()
	rtp.SequenceNumber = rtpSequenceNumber(SN)
	TS, _ := buf.ReadNetUint32()
	rtp.Timestamp = rtpTimestamp(TS)
	SSRC, _ := buf.ReadNetUint32()
	rtp.SSRC = ssrc(SSRC)
	rtp.CSRC = make([]csrc, csrcCount)
	for i := 0; i < csrcCount; i++ {
		CSRC, _ := buf.ReadNetUint32()
		rtp.CSRC[i] = csrc(CSRC)
	}

	return nil
}

func (rtp *rtpPacket) Unpack(rawData []byte) *rtpError {
	buf := streambuf.NewFixed(rawData)
	err := rtp.Header.Unpack(buf)
	if err != nil {
		return err
	}
	rtp.Payload = buf.Bytes()
	return nil
}

func (rtp *rtpPlugin) ParseUDP(pkt *protos.Packet) {
	defer logp.Recover("RTP ParseUdp")
	packetSize := len(pkt.Payload)

	debugf("Parsing packet addressed with %s of length %d.",
		pkt.Tuple.String(), packetSize)

	rtpPkt, err := decodeRTPData(pkt.Payload)
	if err != nil {
		// This means that malformed requests or responses are being sent or
		// that someone is attempting to the RTP port for non-RTP traffic. Both
		// are issues that a monitoring system should report.
		debugf("Decode Error: %s", err.Error())
		return
	}

	event := common.MapStr{}
	event["@timestamp"] = common.Time(pkt.Ts)
	event["type"] = "rtp"
	event["transport"] = "udp"
	//event["src"] = pkt.Tuple.SrcIP.String()
	//	event["dst"] = pkt.Tuple.DstIP.String()
	event["bytes"] = len(pkt.Payload)

	rtpEvent := common.MapStr{}
	event["rtp"] = rtpEvent
	rtpEvent["payload_len"] = len(rtpPkt.Payload)
	rtpHeader := common.MapStr{}
	rtpEvent["header"] = rtpHeader

	rtpHeader["version"] = rtpPkt.Header.Version
	rtpHeader["padded"] = rtpPkt.Header.Padded
	rtpHeader["with_extension"] = rtpPkt.Header.WithExtension
	rtpHeader["marked"] = rtpPkt.Header.Marked
	rtpHeader["payload_type"] = rtpPkt.Header.PayloadType
	rtpHeader["sequence_number"] = rtpPkt.Header.SequenceNumber
	rtpHeader["timestamp"] = rtpPkt.Header.Timestamp
	rtpHeader["ssrc"] = rtpPkt.Header.SSRC
	rtpHeader["csrc"] = rtpPkt.Header.CSRC
	debugf("%s", event)
	rtp.results.PublishTransaction(event)
}

func decodeRTPData(rawData []byte) (rtp *rtpPacket, err error) {
	// Recover from any panics that occur while parsing a packet.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	msg := &rtpPacket{}

	if err := msg.Unpack(rawData); err != nil {
		return nil, err
	}

	return msg, nil
}

func init() {
	protos.Register("rtp", New)
}

func New(
	testMode bool,
	results publish.Transactions,
	cfg *common.Config,
) (protos.Plugin, error) {
	p := &rtpPlugin{}
	config := defaultConfig
	if !testMode {
		if err := cfg.Unpack(&config); err != nil {
			return nil, err
		}
	}

	if err := p.init(results, &config); err != nil {
		return nil, err
	}
	return p, nil
}

func (rtp *rtpPlugin) GetPorts() []int {
	return rtp.ports
}

func (rtp *rtpPlugin) init(results publish.Transactions, config *rtpConfig) error {
	rtp.setFromConfig(config)
	rtp.results = results
	return nil
}

func (rtp *rtpPlugin) setFromConfig(config *rtpConfig) error {
	rtp.ports = config.Ports
	return nil
}
