package socks5

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestParseUDPHeader_IPv4(t *testing.T) {
	// RSV(2) + FRAG(1) + ATYP(1) + IPv4(4) + PORT(2) + DATA
	data := []byte{
		0x00, 0x00, // RSV
		0x00,       // FRAG
		0x01,       // ATYP = IPv4
		8, 8, 8, 8, // IPv4: 8.8.8.8
		0x00, 0x35, // PORT: 53
		0x12, 0x34, 0x56, // payload
	}

	relay := &UDPRelay{sessions: make(map[string]*udpSession)}
	host, port, payload, err := relay.parseUDPHeader(data)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "8.8.8.8" {
		t.Errorf("expected host 8.8.8.8, got %s", host)
	}
	if port != 53 {
		t.Errorf("expected port 53, got %d", port)
	}
	if !bytes.Equal(payload, []byte{0x12, 0x34, 0x56}) {
		t.Errorf("unexpected payload: %v", payload)
	}
}

func TestParseUDPHeader_IPv6(t *testing.T) {
	// RSV(2) + FRAG(1) + ATYP(1) + IPv6(16) + PORT(2) + DATA
	ipv6 := net.ParseIP("2001:4860:4860::8888").To16()
	data := make([]byte, 0, 22+3)
	data = append(data, 0x00, 0x00) // RSV
	data = append(data, 0x00)       // FRAG
	data = append(data, 0x04)       // ATYP = IPv6
	data = append(data, ipv6...)    // IPv6 address
	data = append(data, 0x00, 0x35) // PORT: 53
	data = append(data, 0xAA, 0xBB) // payload

	relay := &UDPRelay{sessions: make(map[string]*udpSession)}
	host, port, payload, err := relay.parseUDPHeader(data)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "2001:4860:4860::8888" {
		t.Errorf("expected host 2001:4860:4860::8888, got %s", host)
	}
	if port != 53 {
		t.Errorf("expected port 53, got %d", port)
	}
	if !bytes.Equal(payload, []byte{0xAA, 0xBB}) {
		t.Errorf("unexpected payload: %v", payload)
	}
}

func TestParseUDPHeader_Domain(t *testing.T) {
	// RSV(2) + FRAG(1) + ATYP(1) + LEN(1) + DOMAIN + PORT(2) + DATA
	domain := "dns.google"
	data := make([]byte, 0)
	data = append(data, 0x00, 0x00)     // RSV
	data = append(data, 0x00)           // FRAG
	data = append(data, 0x03)           // ATYP = Domain
	data = append(data, byte(len(domain))) // Domain length
	data = append(data, []byte(domain)...) // Domain
	data = append(data, 0x01, 0xBB)     // PORT: 443
	data = append(data, 0xDE, 0xAD)     // payload

	relay := &UDPRelay{sessions: make(map[string]*udpSession)}
	host, port, payload, err := relay.parseUDPHeader(data)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != domain {
		t.Errorf("expected host %s, got %s", domain, host)
	}
	if port != 443 {
		t.Errorf("expected port 443, got %d", port)
	}
	if !bytes.Equal(payload, []byte{0xDE, 0xAD}) {
		t.Errorf("unexpected payload: %v", payload)
	}
}

func TestParseUDPHeader_Fragmentation(t *testing.T) {
	data := []byte{
		0x00, 0x00, // RSV
		0x01,       // FRAG = 1 (not supported)
		0x01,       // ATYP = IPv4
		8, 8, 8, 8,
		0x00, 0x35,
	}

	relay := &UDPRelay{sessions: make(map[string]*udpSession)}
	_, _, _, err := relay.parseUDPHeader(data)

	if err == nil {
		t.Error("expected error for fragmentation, got nil")
	}
}

func TestParseUDPHeader_TooShort(t *testing.T) {
	data := []byte{0x00, 0x00, 0x00, 0x01, 8, 8} // too short

	relay := &UDPRelay{sessions: make(map[string]*udpSession)}
	_, _, _, err := relay.parseUDPHeader(data)

	if err == nil {
		t.Error("expected error for short packet, got nil")
	}
}

func TestBuildUDPHeader_IPv4(t *testing.T) {
	relay := &UDPRelay{}
	payload := []byte{0x12, 0x34}
	result := relay.buildUDPHeader("8.8.8.8", 53, payload)

	// RSV(2) + FRAG(1) + ATYP(1) + IPv4(4) + PORT(2) + DATA
	if len(result) != 10+len(payload) {
		t.Fatalf("expected length %d, got %d", 10+len(payload), len(result))
	}

	if result[0] != 0x00 || result[1] != 0x00 {
		t.Error("RSV should be 0x0000")
	}
	if result[2] != 0x00 {
		t.Error("FRAG should be 0x00")
	}
	if result[3] != AddrIPv4 {
		t.Errorf("ATYP should be IPv4 (0x01), got 0x%02x", result[3])
	}
	if !bytes.Equal(result[4:8], []byte{8, 8, 8, 8}) {
		t.Error("IPv4 address mismatch")
	}
	port := binary.BigEndian.Uint16(result[8:10])
	if port != 53 {
		t.Errorf("expected port 53, got %d", port)
	}
	if !bytes.Equal(result[10:], payload) {
		t.Error("payload mismatch")
	}
}

func TestBuildUDPHeader_IPv6(t *testing.T) {
	relay := &UDPRelay{}
	payload := []byte{0xAB}
	result := relay.buildUDPHeader("2001:4860:4860::8888", 443, payload)

	// RSV(2) + FRAG(1) + ATYP(1) + IPv6(16) + PORT(2) + DATA
	if len(result) != 22+len(payload) {
		t.Fatalf("expected length %d, got %d", 22+len(payload), len(result))
	}

	if result[3] != AddrIPv6 {
		t.Errorf("ATYP should be IPv6 (0x04), got 0x%02x", result[3])
	}

	expectedIP := net.ParseIP("2001:4860:4860::8888").To16()
	if !bytes.Equal(result[4:20], expectedIP) {
		t.Error("IPv6 address mismatch")
	}

	port := binary.BigEndian.Uint16(result[20:22])
	if port != 443 {
		t.Errorf("expected port 443, got %d", port)
	}
}

func TestBuildUDPHeader_Domain(t *testing.T) {
	relay := &UDPRelay{}
	payload := []byte{0xCA, 0xFE}
	domain := "example.com"
	result := relay.buildUDPHeader(domain, 80, payload)

	// RSV(2) + FRAG(1) + ATYP(1) + LEN(1) + DOMAIN + PORT(2) + DATA
	expectedLen := 5 + len(domain) + 2 + len(payload)
	if len(result) != expectedLen {
		t.Fatalf("expected length %d, got %d", expectedLen, len(result))
	}

	if result[3] != AddrDomain {
		t.Errorf("ATYP should be Domain (0x03), got 0x%02x", result[3])
	}
	if int(result[4]) != len(domain) {
		t.Errorf("domain length should be %d, got %d", len(domain), result[4])
	}
	if string(result[5:5+len(domain)]) != domain {
		t.Error("domain mismatch")
	}

	portOffset := 5 + len(domain)
	port := binary.BigEndian.Uint16(result[portOffset : portOffset+2])
	if port != 80 {
		t.Errorf("expected port 80, got %d", port)
	}
}

func TestParseAndBuild_Roundtrip(t *testing.T) {
	testCases := []struct {
		name    string
		host    string
		port    int
		payload []byte
	}{
		{"IPv4", "1.2.3.4", 53, []byte("hello")},
		{"IPv6", "::1", 443, []byte{0x01, 0x02}},
		{"Domain", "dns.google", 853, []byte("test data")},
	}

	relay := &UDPRelay{sessions: make(map[string]*udpSession)}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			built := relay.buildUDPHeader(tc.host, tc.port, tc.payload)
			parsedHost, parsedPort, parsedPayload, err := relay.parseUDPHeader(built)

			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			// 对于 ::1 这种简短的 IPv6，Go 会规范化为 "::1"
			expectedHost := tc.host
			if net.ParseIP(tc.host) != nil {
				expectedHost = net.ParseIP(tc.host).String()
			}

			if parsedHost != expectedHost {
				t.Errorf("host mismatch: expected %s, got %s", expectedHost, parsedHost)
			}
			if parsedPort != tc.port {
				t.Errorf("port mismatch: expected %d, got %d", tc.port, parsedPort)
			}
			if !bytes.Equal(parsedPayload, tc.payload) {
				t.Errorf("payload mismatch")
			}
		})
	}
}
