// +build !integration

// Unit tests and benchmarks for the rtp package.
//
// The byte array test data was generated from pcap files using the gopacket
// test_creator.py script contained in the gopacket repository. The script was
// modified to drop the Ethernet, IP, and UDP headers from the byte arrays
// (skip the first 42 bytes).
//

package rtp

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/elastic/beats/packetbeat/protos"
	"github.com/elastic/beats/packetbeat/publish"

	"github.com/elastic/beats/libbeat/common"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/stretchr/testify/assert"
)

// Verify that the interface for UDP has been satisfied.
var _ protos.UDPPlugin = &rtpPlugin{}

const (
	serverIP   = "192.168.0.1"
	serverPort = 54467
	clientIP   = "10.0.0.1"
	clientPort = 34898
)

var (
	forward = common.NewIPPortTuple(4,
		net.ParseIP(serverIP), serverPort,
		net.ParseIP(clientIP), clientPort)
	reverse = common.NewIPPortTuple(4,
		net.ParseIP(clientIP), clientPort,
		net.ParseIP(serverIP), serverPort)
)

type rtpTestMessage struct {
	Name           string
	Version        uint8
	Padded         bool
	WithExtension  bool
	Marked         bool
	PayloadType    uint8
	SequenceNumber rtpSequenceNumber
	Timestamp      rtpTimestamp
	SSRC           ssrc
	CSRC           []csrc
	PayloadLength  int
	data           []byte
}

// RTP messages for testing. When adding a new test message, add it to the
// messages array and create a new benchmark test for the message.
var (
	// An array of all test messages.
	messages = []rtpTestMessage{
		rtpH264,
	}

	rtpH264 = rtpTestMessage{
		Name:           "RTP with H264 Payload",
		Version:        2,
		Padded:         false,
		WithExtension:  false,
		Marked:         false,
		PayloadType:    96,
		SequenceNumber: 19798,
		Timestamp:      4146225580,
		SSRC:           0x50b5958c,
		CSRC:           make([]csrc, 0),
		PayloadLength:  1372,
		// hexdump -v -e '16/1 "0x%02x, "' -e '"\n"'
		data: []byte{
			0x80, 0x60, 0x4d, 0x56, 0xf7, 0x22, 0x61, 0xac, 0x50, 0xb5, 0x95, 0x8c, 0x7c, 0x81, 0x9a, 0x0d,
			0x0d, 0x6a, 0xeb, 0xff, 0xb7, 0x7a, 0x58, 0x1f, 0x0d, 0x41, 0x12, 0x21, 0x7a, 0x09, 0x8e, 0xa5,
			0x51, 0x24, 0x6d, 0xb6, 0x71, 0x0f, 0xfe, 0xef, 0x26, 0x71, 0xb8, 0x3d, 0x92, 0xac, 0xdf, 0xd7,
			0xb7, 0xd4, 0xd9, 0x63, 0x84, 0x1a, 0x90, 0x45, 0xee, 0x64, 0x25, 0x89, 0xc7, 0xe4, 0xc3, 0xca,
			0x11, 0xc3, 0x28, 0x5e, 0xb5, 0x80, 0x45, 0x6f, 0xca, 0x4e, 0x38, 0xad, 0xa3, 0xc5, 0x2b, 0x37,
			0xc2, 0xfa, 0x4f, 0x1b, 0x50, 0xe8, 0x70, 0xb7, 0x7a, 0x55, 0xaa, 0x05, 0xaa, 0x2b, 0x52, 0x4e,
			0x29, 0xde, 0x07, 0xe9, 0x7b, 0x0c, 0x7a, 0x5b, 0xc0, 0x9b, 0xbe, 0x2f, 0xb9, 0xe1, 0x55, 0x34,
			0x28, 0x7c, 0xd5, 0x39, 0xfa, 0x88, 0xbb, 0x3a, 0x1a, 0x6c, 0xb6, 0xb7, 0xe9, 0xa4, 0x13, 0x93,
			0x43, 0xb1, 0x2a, 0x11, 0xe7, 0xf9, 0x19, 0x96, 0x7c, 0x23, 0x9d, 0x2c, 0xe8, 0xad, 0xa0, 0x9c,
			0x87, 0xf7, 0x25, 0x3f, 0xef, 0x88, 0x95, 0x81, 0xfe, 0x47, 0x80, 0xbb, 0xfd, 0x90, 0x47, 0x4c,
			0xbb, 0x61, 0x5f, 0x60, 0x46, 0xe5, 0x5d, 0xb9, 0x6f, 0xca, 0x4e, 0x67, 0x73, 0x30, 0xf7, 0x3e,
			0x99, 0xea, 0x4d, 0x3a, 0x41, 0x23, 0x64, 0x8d, 0xd6, 0x55, 0x1e, 0xce, 0xc6, 0xac, 0x11, 0x17,
			0x3c, 0x7c, 0x1d, 0x09, 0xc6, 0x6a, 0x65, 0xcc, 0x3b, 0x4d, 0xa0, 0x10, 0x8a, 0xa3, 0x45, 0xed,
			0x27, 0xff, 0xf3, 0x99, 0xd4, 0x70, 0xe7, 0x73, 0x94, 0x8f, 0xcf, 0x3c, 0x94, 0xa6, 0x1d, 0x03,
			0x6d, 0xcf, 0x77, 0x57, 0x1f, 0xa0, 0xd9, 0x9a, 0x9c, 0x4b, 0xa3, 0x08, 0x0e, 0xbe, 0x05, 0x32,
			0xdc, 0x24, 0x80, 0xf1, 0x89, 0xed, 0x0e, 0x7f, 0xfe, 0xd1, 0xf2, 0x37, 0x0d, 0xaf, 0xc8, 0xaf,
			0x0b, 0xfc, 0xef, 0x10, 0x05, 0xf4, 0xd9, 0xde, 0x50, 0xb5, 0x60, 0xb9, 0xc9, 0xd7, 0xae, 0x14,
			0xe3, 0xe0, 0x93, 0x03, 0xf4, 0x75, 0x9a, 0xb5, 0x56, 0x4d, 0x7e, 0xcb, 0xd6, 0xd2, 0x05, 0x4e,
			0xe1, 0x5c, 0xf4, 0x23, 0x3a, 0xe4, 0x0e, 0xb3, 0x40, 0xc9, 0x18, 0xaf, 0xe5, 0xad, 0xd3, 0xcb,
			0x21, 0x36, 0xcd, 0x7f, 0x5f, 0x90, 0x0e, 0x30, 0x0c, 0x06, 0xc9, 0x84, 0x9a, 0xcd, 0x0f, 0x8e,
			0x49, 0xbb, 0xf4, 0x3a, 0x45, 0x16, 0xb3, 0x74, 0xf7, 0xc3, 0xcd, 0x9b, 0xc2, 0x46, 0x8c, 0x9d,
			0x6e, 0x33, 0x9f, 0x08, 0x6e, 0x61, 0xc7, 0x89, 0x40, 0xac, 0x50, 0x4a, 0xf9, 0xaf, 0xc8, 0x90,
			0xe3, 0xf9, 0x3b, 0xc7, 0xc9, 0xc6, 0xe2, 0x23, 0xe8, 0xa7, 0x66, 0x92, 0x62, 0x32, 0x0a, 0x30,
			0x99, 0x8f, 0xdb, 0xc4, 0x36, 0xc9, 0x03, 0xb5, 0xc3, 0x56, 0x3d, 0xe9, 0x50, 0x43, 0x6c, 0x38,
			0x10, 0xf9, 0x3d, 0x84, 0x7f, 0xbc, 0xfe, 0xa2, 0x54, 0xaa, 0x25, 0x8c, 0x5a, 0x0f, 0x39, 0xf7,
			0xc7, 0xc2, 0x98, 0x2d, 0x93, 0xe3, 0xaa, 0xe6, 0xc0, 0xc1, 0xd0, 0x06, 0xea, 0xee, 0x87, 0xe1,
			0xb2, 0x2c, 0x66, 0xc4, 0x00, 0x8d, 0x70, 0xdc, 0x6d, 0x2f, 0xd9, 0x67, 0x54, 0x8e, 0x71, 0xb3,
			0x43, 0x33, 0x1c, 0xd5, 0x1c, 0xf5, 0xee, 0xd0, 0xe7, 0xbe, 0x9f, 0xc8, 0x26, 0xc5, 0xd4, 0x0d,
			0x77, 0x4f, 0xd7, 0xe4, 0xb3, 0x19, 0xd0, 0x11, 0x89, 0xc4, 0xe3, 0x15, 0x5b, 0xfe, 0xff, 0x2a,
			0xb1, 0x0c, 0x00, 0x6c, 0x5b, 0xf2, 0xbe, 0x66, 0x72, 0x3f, 0x9c, 0x50, 0x29, 0xf8, 0x5c, 0x08,
			0x75, 0xa8, 0x3b, 0x6b, 0x50, 0x81, 0x70, 0x31, 0x74, 0xdd, 0x73, 0xd7, 0x4b, 0xb4, 0xc7, 0xc3,
			0x2f, 0xa3, 0x94, 0x27, 0xbb, 0x69, 0x6f, 0x14, 0xc3, 0xec, 0x99, 0x68, 0xde, 0x6a, 0xe9, 0xf6,
			0x0d, 0x7b, 0x72, 0xbc, 0x74, 0x2d, 0xd1, 0x8d, 0xb2, 0x95, 0x2b, 0xa3, 0xbc, 0xfa, 0x0e, 0x7c,
			0x36, 0x4b, 0xf5, 0x2f, 0x97, 0x5d, 0x0d, 0x36, 0x2e, 0xd3, 0xf7, 0x3b, 0xd0, 0xaa, 0x6a, 0x3f,
			0x66, 0x80, 0x37, 0x83, 0x27, 0xa9, 0x51, 0x6b, 0xff, 0x43, 0xf0, 0xd3, 0x2e, 0x2a, 0x80, 0xda,
			0xa0, 0xb0, 0x85, 0x86, 0x0d, 0x97, 0x47, 0x38, 0x00, 0x95, 0xaf, 0xb7, 0x1e, 0xd9, 0xd4, 0xf7,
			0x6a, 0x18, 0x69, 0x29, 0xd8, 0x94, 0xa6, 0x28, 0x4b, 0xfb, 0x19, 0x92, 0x8a, 0xa7, 0x69, 0x0f,
			0xdc, 0xe4, 0xa8, 0x86, 0x64, 0x48, 0x53, 0x52, 0xe1, 0xc1, 0xdb, 0xfb, 0x1f, 0x7d, 0xc0, 0xc6,
			0x4b, 0x4e, 0xcd, 0x97, 0xcb, 0xb7, 0xc8, 0x81, 0xb7, 0x5f, 0x0a, 0xf6, 0x78, 0x5a, 0xf7, 0xd0,
			0xeb, 0x93, 0x87, 0x93, 0x30, 0xc8, 0xdd, 0x04, 0x00, 0xeb, 0x71, 0x19, 0x5f, 0x0f, 0xca, 0x4c,
			0xd1, 0x8f, 0x37, 0x45, 0xf7, 0x5d, 0xaa, 0x47, 0xb4, 0xa1, 0x05, 0x74, 0x56, 0xd7, 0x17, 0x65,
			0xbd, 0x1d, 0x53, 0x4d, 0x79, 0x35, 0xaa, 0x9f, 0x0d, 0xd2, 0xdb, 0x04, 0x47, 0x02, 0x69, 0x5c,
			0xbb, 0xb3, 0xdf, 0x25, 0xd4, 0x1f, 0xb0, 0x16, 0xa8, 0x9b, 0xa5, 0xc9, 0xfe, 0xe3, 0x20, 0x6a,
			0x63, 0xd0, 0x26, 0x8e, 0x30, 0x75, 0x86, 0x2b, 0x1e, 0x1a, 0x55, 0x96, 0xbc, 0xbc, 0x79, 0x4b,
			0x80, 0x7d, 0x13, 0x2e, 0xa9, 0x31, 0x72, 0x56, 0x01, 0x7c, 0x3f, 0x1d, 0x1d, 0xf5, 0x8f, 0xf2,
			0x88, 0xe3, 0x8e, 0x8a, 0x75, 0xf5, 0x0b, 0x8c, 0x22, 0x47, 0x1b, 0x93, 0xbb, 0xcc, 0xd1, 0x82,
			0xd3, 0x66, 0x76, 0x7d, 0x78, 0xbc, 0x07, 0x27, 0x59, 0x19, 0x7d, 0x06, 0x2c, 0x6a, 0xe6, 0xcb,
			0xac, 0x01, 0x0a, 0xbc, 0x40, 0x9d, 0x29, 0x1d, 0xad, 0xed, 0x17, 0xb1, 0x65, 0xa4, 0xf2, 0xc7,
			0xa5, 0x69, 0x6b, 0xcf, 0xc9, 0xb5, 0x57, 0xc0, 0x63, 0xa5, 0xac, 0x0e, 0xd4, 0x93, 0xa3, 0x15,
			0x0e, 0xe0, 0x2a, 0x6a, 0xa0, 0xed, 0x4b, 0x50, 0xb1, 0x32, 0x58, 0xe8, 0x1a, 0x9b, 0x35, 0x67,
			0x1b, 0x3b, 0x38, 0x32, 0xf7, 0xac, 0x0e, 0x2e, 0x77, 0x92, 0x25, 0x2f, 0x17, 0x63, 0x59, 0x8f,
			0x65, 0x10, 0x84, 0x2b, 0x01, 0x33, 0x97, 0x4c, 0x20, 0x3a, 0x84, 0x35, 0xde, 0x5e, 0xc9, 0x7b,
			0x53, 0x0e, 0x50, 0x00, 0xec, 0xd4, 0xeb, 0x9c, 0x05, 0xd1, 0xdd, 0x5c, 0x34, 0xd3, 0x29, 0xf0,
			0x77, 0x8a, 0x70, 0xa2, 0xfa, 0x82, 0x76, 0x7e, 0x81, 0x7e, 0x21, 0x60, 0xd7, 0x66, 0xd3, 0x23,
			0x75, 0x33, 0x5e, 0x45, 0xda, 0x46, 0x37, 0x3a, 0x93, 0x63, 0xe8, 0x5e, 0xe5, 0x94, 0x4f, 0x6f,
			0x04, 0x79, 0x34, 0xb8, 0x02, 0x38, 0x90, 0x45, 0x1e, 0x25, 0xc0, 0xf8, 0xaa, 0xb2, 0xbd, 0x40,
			0x24, 0x02, 0xf2, 0x1c, 0x35, 0xe3, 0xd9, 0x7b, 0x42, 0xa7, 0xe4, 0xb8, 0x4e, 0xd4, 0xb8, 0x14,
			0xa8, 0xae, 0xac, 0x50, 0xd6, 0xc2, 0xae, 0xfc, 0x94, 0x9b, 0x85, 0xab, 0x62, 0x2a, 0x76, 0xa5,
			0xc6, 0xc7, 0xab, 0x43, 0x24, 0x33, 0xbc, 0x88, 0xc7, 0xc4, 0x0e, 0x89, 0x0f, 0x76, 0x08, 0xdd,
			0x6a, 0x2b, 0x22, 0x37, 0xcb, 0x47, 0x42, 0xdd, 0x83, 0x14, 0x7a, 0x8d, 0xc7, 0x4c, 0x9e, 0xa8,
			0xec, 0x87, 0x12, 0x21, 0x79, 0xcb, 0x5e, 0x64, 0x0d, 0x1d, 0xa0, 0xc0, 0x5e, 0xdc, 0x43, 0xe8,
			0x7a, 0x2e, 0x94, 0x08, 0x16, 0x1d, 0x60, 0x33, 0x75, 0x3e, 0xcc, 0x92, 0xc2, 0x83, 0x5e, 0xa5,
			0xa6, 0xd4, 0x5e, 0x7e, 0x1b, 0x93, 0xec, 0x2d, 0x72, 0x43, 0x87, 0xe0, 0x9b, 0x4b, 0x40, 0xac,
			0x59, 0x34, 0x8b, 0x02, 0x0e, 0xc8, 0xb8, 0xc5, 0xc4, 0x89, 0xc8, 0x90, 0x14, 0xf6, 0xa3, 0x0f,
			0x32, 0xc5, 0x88, 0xcf, 0xa6, 0x39, 0x1e, 0x93, 0x11, 0xc4, 0x82, 0x9d, 0x13, 0x5f, 0x81, 0xd8,
			0x89, 0xdd, 0xaf, 0x0d, 0xc6, 0xc9, 0xd7, 0xb0, 0x78, 0x76, 0xa7, 0x3e, 0x93, 0x7e, 0x36, 0x94,
			0xbd, 0x5d, 0xb9, 0xef, 0x50, 0x01, 0xdc, 0xbc, 0x4d, 0xaa, 0xa0, 0x55, 0xf3, 0x06, 0xb0, 0x2f,
			0xff, 0xd7, 0x55, 0x80, 0x08, 0x4e, 0xab, 0x7a, 0xf5, 0x33, 0xb6, 0xb8, 0xf9, 0xef, 0x1b, 0x8d,
			0x7f, 0x6a, 0x86, 0x7a, 0x11, 0x0f, 0xe1, 0xc2, 0x03, 0xbd, 0x28, 0x39, 0x14, 0xcf, 0xee, 0xe9,
			0x25, 0xa2, 0x2e, 0x46, 0x85, 0x28, 0x9e, 0xb7, 0xc2, 0xd7, 0x51, 0x23, 0xdd, 0x65, 0xfb, 0xd4,
			0xbc, 0xc8, 0xc7, 0x68, 0xd4, 0x42, 0x68, 0xd4, 0x66, 0x33, 0xb0, 0x77, 0x2e, 0x64, 0xf1, 0xc4,
			0x6e, 0x82, 0xef, 0x73, 0x67, 0x62, 0xd8, 0xee, 0x1c, 0x0f, 0xd6, 0x57, 0x51, 0x43, 0x27, 0x33,
			0x29, 0x75, 0x71, 0x2d, 0xb8, 0xd4, 0x0b, 0x37, 0x04, 0xb4, 0x95, 0xfe, 0x26, 0xd8, 0x0b, 0x29,
			0xb9, 0x84, 0xdc, 0x65, 0x06, 0xc0, 0xb8, 0x89, 0xcc, 0x26, 0x12, 0x20, 0x5e, 0xb0, 0xd8, 0xca,
			0xe7, 0x6d, 0x68, 0x68, 0xb0, 0x9c, 0x6a, 0x66, 0xab, 0xba, 0xf4, 0xd6, 0xbf, 0xd8, 0x97, 0x2d,
			0x2d, 0x84, 0x49, 0xa1, 0xc1, 0xe8, 0xbe, 0x01, 0xca, 0xd4, 0x2d, 0x34, 0x39, 0xe2, 0xb0, 0x63,
			0xeb, 0x39, 0xeb, 0x5e, 0x6e, 0xed, 0xd6, 0xcd, 0xc0, 0x7f, 0x47, 0x4d, 0x48, 0xd7, 0xf8, 0x59,
			0x45, 0x5a, 0xb9, 0x24, 0xdb, 0x0e, 0xf1, 0x4f, 0xc0, 0xce, 0xea, 0xdc, 0x61, 0xed, 0xdf, 0xa8,
			0xbe, 0xd0, 0x6d, 0x5c, 0x7e, 0x30, 0x48, 0x56, 0x31, 0xda, 0xc5, 0x1e, 0x6e, 0x3f, 0xd5, 0xb7,
			0x33, 0xab, 0x73, 0xee, 0x5a, 0x68, 0xb6, 0x7e, 0x52, 0x35, 0x8e, 0x0b, 0x88, 0x88, 0x64, 0xa7,
			0x0d, 0x30, 0x5d, 0x0e, 0x31, 0x90, 0x93, 0xc0, 0x56, 0xf2, 0x06, 0xb2, 0x24, 0x97, 0x9f, 0x05,
			0x7c, 0xef, 0x1e, 0x5b, 0x47, 0x65, 0x2b, 0x26, 0x2b, 0x41, 0xec, 0xb2, 0x95, 0xa4, 0x76, 0x16,
			0x25, 0xc8, 0x7b, 0x2b, 0x0d, 0xd6, 0x98, 0x41, 0xad, 0xf2, 0x2b, 0x6b, 0xbf, 0x04, 0xf1, 0x34,
			0xc5, 0x05, 0xae, 0x30, 0x2c, 0x46, 0xcf, 0x0a, 0x2e, 0xb2, 0xd6, 0x3f, 0xb5, 0xdb, 0xef, 0xc3,
			0x48, 0x25, 0x67, 0xb5, 0x1c, 0xc0, 0x10, 0xf7, 0xd0, 0x03, 0x47, 0xa4, 0xb4, 0x37, 0x7b, 0xed,
			0xc7, 0x8b, 0xbf, 0xb9, 0x8a, 0x8d, 0xf6, 0xf1, 0x6f, 0x1f, 0xd2, 0x06, 0x9f, 0xd5, 0x54, 0xb7,
			0xb8, 0x9d, 0x7f, 0xeb, 0x6b, 0xc2, 0x0d, 0x7b,
		},
	}
)

func newRTP(verbose bool) *rtpPlugin {
	if verbose {
		logp.LogInit(logp.LOG_DEBUG, "", false, true, []string{"rtp"})
	} else {
		logp.LogInit(logp.LOG_EMERG, "", false, true, []string{"rtp"})
	}

	results := &publish.ChanTransactions{make(chan common.MapStr, 100)}
	cfg, _ := common.NewConfigFrom(map[string]interface{}{
		"ports": []int{serverPort},
	})
	rtp, err := New(false, results, cfg)
	if err != nil {
		panic(err)
	}

	return rtp.(*rtpPlugin)
}

func newPacket(t common.IPPortTuple, payload []byte) *protos.Packet {
	return &protos.Packet{
		Ts:      time.Now(),
		Tuple:   t,
		Payload: payload,
	}
}

// Verify that an empty packet is safely handled (no panics).
func TestParseUdp_emptyPacket(t *testing.T) {
	rtp := newRTP(testing.Verbose())
	packet := newPacket(forward, []byte{})
	rtp.ParseUDP(packet)
	client := rtp.results.(*publish.ChanTransactions)
	close(client.Channel)
	assert.Nil(t, <-client.Channel, "No result should have been published.")
}

// Verify that a malformed packet is safely handled (no panics).
func TestParseUdp_malformedPacket(t *testing.T) {
	rtp := newRTP(testing.Verbose())
	garbage := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	packet := newPacket(forward, garbage)
	rtp.ParseUDP(packet)
	// As a future addition, a malformed message should publish a result.
}

// Verify all RTP test messages are parsed correctly.
func TestParseUdp_allTestMessages(t *testing.T) {
	rtp := newRTP(testing.Verbose())
	for _, q := range messages {
		t.Logf("Testing with query for %s", q.Name)
		parseUDPRequestResponse(t, rtp, q)
	}
}

// Benchmarks UDP parsing for the given test message.
func benchmarkUDP(b *testing.B, q rtpTestMessage) {
	rtp := newRTP(false)
	for i := 0; i < b.N; i++ {
		packet := newPacket(forward, q.data)
		rtp.ParseUDP(packet)

		client := rtp.results.(*publish.ChanTransactions)
		<-client.Channel
	}
}

// Benchmark UDP parsing against each test message.
func BenchmarkUdpElasticA(b *testing.B) { benchmarkUDP(b, rtpH264) }

// Benchmark that runs with parallelism to help find concurrency related
// issues. To run with parallelism, the 'go test' cpu flag must be set
// greater than 1, otherwise it just runs concurrently but not in parallel.
func BenchmarkParallelUdpParse(b *testing.B) {
	rand.Seed(22)
	numMessages := len(messages)
	rtp := newRTP(false)
	client := rtp.results.(*publish.ChanTransactions)

	// Drain the results channal while the test is running.
	go func() {
		totalMessages := 0
		for r := range client.Channel {
			_ = r
			totalMessages++
		}
		fmt.Printf("Parsed %d messages.\n", totalMessages)
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Each iteration parses one message, either a request or a response.
		// The request and response could be parsed on different goroutines.
		for pb.Next() {
			q := messages[rand.Intn(numMessages)]
			var packet *protos.Packet
			packet = newPacket(reverse, q.data)
			rtp.ParseUDP(packet)
		}
	})

	defer close(client.Channel)
}

// expectResult returns one MapStr result from the RTP results channel. If
// no result is available then the test fails.
func expectResult(t testing.TB, rtp *rtpPlugin) common.MapStr {
	client := rtp.results.(*publish.ChanTransactions)
	select {
	case result := <-client.Channel:
		return result
	default:
		t.Error("Expected a result to be published.")
	}
	return nil
}

// Retrieves a map value. The key should be the full dotted path to the element.
func mapValue(t testing.TB, m common.MapStr, key string) interface{} {
	return mapValueHelper(t, m, strings.Split(key, "."))
}

// Retrieves nested MapStr values.
func mapValueHelper(t testing.TB, m common.MapStr, keys []string) interface{} {
	key := keys[0]
	if len(keys) == 1 {
		return m[key]
	}

	if len(keys) > 1 {
		value, exists := m[key]
		if !exists {
			t.Fatalf("%s is missing from MapStr %v.", key, m)
		}

		switch typ := value.(type) {
		default:
			t.Fatalf("Expected %s to return a MapStr but got %v.", key, value)
		case common.MapStr:
			return mapValueHelper(t, typ, keys[1:])
		case []common.MapStr:
			var values []interface{}
			for _, m := range typ {
				values = append(values, mapValueHelper(t, m, keys[1:]))
			}
			return values
		}
	}

	panic("mapValueHelper cannot be called with an empty array of keys")
}

// parseUdpRequestResponse parses a request then a response packet and validates
// the published result.
func parseUDPRequestResponse(t testing.TB, rtp *rtpPlugin, q rtpTestMessage) {
	packet := newPacket(forward, q.data)
	rtp.ParseUDP(packet)

	m := expectResult(t, rtp)

	assert.Equal(t, "udp", mapValue(t, m, "transport"))
	assert.Equal(t, len(q.data), mapValue(t, m, "bytes"))

	assert.Equal(t, uint8(2), mapValue(t, m, "rtp.header.version"), "Version")
	assert.Equal(t, q.Padded, mapValue(t, m, "rtp.header.padded"), "Padded")
	assert.Equal(t, q.WithExtension, mapValue(t, m, "rtp.header.with_extension"), "Extension")
	assert.Equal(t, q.Marked, mapValue(t, m, "rtp.header.marked"), "Marker")
	assert.Equal(t, q.PayloadType, mapValue(t, m, "rtp.header.payload_type"), "Payload Type")
	assert.Equal(t, q.SequenceNumber, mapValue(t, m, "rtp.header.sequence_number"), "Sequence Number")
	assert.Equal(t, q.Timestamp, mapValue(t, m, "rtp.header.timestamp"), "Timestamp")
	assert.Equal(t, q.SSRC, mapValue(t, m, "rtp.header.ssrc"), "SSRC")
	assert.Equal(t, q.PayloadLength, mapValue(t, m, "rtp.payload_len"), "Payload Length")
	//	assert.Equal(t, q.CSRC, mapValue(t, m, "rtp.header.csrc", "CSRC count"))

}
