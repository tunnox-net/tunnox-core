package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"
)

// SOCKS5 constants
const (
	Version    = 0x05
	AuthNone   = 0x00
	CmdUDPAssoc = 0x03
	AddrIPv4   = 0x01
	RepSuccess = 0x00
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test_udp_associate <socks5_addr>")
		fmt.Println("Example: test_udp_associate 127.0.0.1:1080")
		os.Exit(1)
	}

	socks5Addr := os.Args[1]
	fmt.Printf("Testing SOCKS5 UDP ASSOCIATE on %s\n", socks5Addr)

	// 1. Connect to SOCKS5 server
	conn, err := net.DialTimeout("tcp", socks5Addr, 5*time.Second)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to SOCKS5 server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("✓ Connected to SOCKS5 server")

	// 2. Handshake - send version and auth methods
	_, err = conn.Write([]byte{Version, 1, AuthNone})
	if err != nil {
		fmt.Printf("ERROR: Failed to send handshake: %v\n", err)
		os.Exit(1)
	}

	// Read auth response
	buf := make([]byte, 2)
	_, err = conn.Read(buf)
	if err != nil {
		fmt.Printf("ERROR: Failed to read auth response: %v\n", err)
		os.Exit(1)
	}
	if buf[0] != Version || buf[1] != AuthNone {
		fmt.Printf("ERROR: Unexpected auth response: version=%d, method=%d\n", buf[0], buf[1])
		os.Exit(1)
	}
	fmt.Println("✓ Handshake successful (no auth required)")

	// 3. Send UDP ASSOCIATE request
	// Request format: VER CMD RSV ATYP DST.ADDR DST.PORT
	// For UDP ASSOCIATE, DST.ADDR and DST.PORT are the client's address/port (0.0.0.0:0 is common)
	request := []byte{
		Version,    // VER
		CmdUDPAssoc, // CMD = UDP ASSOCIATE
		0x00,       // RSV
		AddrIPv4,   // ATYP = IPv4
		0, 0, 0, 0, // DST.ADDR = 0.0.0.0
		0, 0,       // DST.PORT = 0
	}
	_, err = conn.Write(request)
	if err != nil {
		fmt.Printf("ERROR: Failed to send UDP ASSOCIATE request: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Sent UDP ASSOCIATE request")

	// 4. Read UDP ASSOCIATE response
	response := make([]byte, 10) // VER REP RSV ATYP BND.ADDR(4) BND.PORT(2)
	_, err = conn.Read(response)
	if err != nil {
		fmt.Printf("ERROR: Failed to read UDP ASSOCIATE response: %v\n", err)
		os.Exit(1)
	}

	if response[0] != Version {
		fmt.Printf("ERROR: Invalid version in response: %d\n", response[0])
		os.Exit(1)
	}

	rep := response[1]
	if rep != RepSuccess {
		repNames := map[byte]string{
			0x01: "general SOCKS server failure",
			0x02: "connection not allowed by ruleset",
			0x03: "network unreachable",
			0x04: "host unreachable",
			0x05: "connection refused",
			0x06: "TTL expired",
			0x07: "command not supported",
			0x08: "address type not supported",
		}
		repName, ok := repNames[rep]
		if !ok {
			repName = "unknown"
		}
		fmt.Printf("ERROR: UDP ASSOCIATE failed with REP=%d (%s)\n", rep, repName)
		if rep == 0x07 {
			fmt.Println("\n>>> The server returned 'command not supported'!")
			fmt.Println(">>> This means UDPRelayCreator is nil in the SOCKS5 listener.")
			fmt.Println(">>> Check if SetUDPRelayCreator() was called on the listener.")
		}
		os.Exit(1)
	}

	// Parse BND.ADDR and BND.PORT
	atyp := response[3]
	if atyp != AddrIPv4 {
		fmt.Printf("ERROR: Unexpected address type: %d (expected IPv4)\n", atyp)
		os.Exit(1)
	}

	bindAddr := net.IP(response[4:8])
	bindPort := binary.BigEndian.Uint16(response[8:10])

	fmt.Printf("✓ UDP ASSOCIATE successful!\n")
	fmt.Printf("  Bind Address: %s:%d\n", bindAddr, bindPort)

	// 5. Test UDP relay
	fmt.Println("\n--- Testing UDP relay ---")

	udpAddr := &net.UDPAddr{IP: bindAddr, Port: int(bindPort)}
	udpConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to UDP relay: %v\n", err)
		os.Exit(1)
	}
	defer udpConn.Close()
	fmt.Printf("✓ Connected to UDP relay at %s:%d\n", bindAddr, bindPort)

	// Build SOCKS5 UDP request to 8.8.8.8:53 (DNS)
	// Format: RSV(2) FRAG ATYP DST.ADDR DST.PORT DATA
	dnsQuery := buildDNSQuery("example.com")
	udpRequest := []byte{
		0, 0,       // RSV
		0,          // FRAG
		AddrIPv4,   // ATYP
		8, 8, 8, 8, // DST.ADDR = 8.8.8.8
		0, 53,      // DST.PORT = 53
	}
	udpRequest = append(udpRequest, dnsQuery...)

	_, err = udpConn.Write(udpRequest)
	if err != nil {
		fmt.Printf("ERROR: Failed to send UDP packet: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Sent DNS query via UDP relay")

	// Wait for response
	udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	respBuf := make([]byte, 4096)
	n, err := udpConn.Read(respBuf)
	if err != nil {
		fmt.Printf("ERROR: Failed to read UDP response: %v\n", err)
		fmt.Println("\n>>> The UDP relay might not be forwarding data correctly.")
		fmt.Println(">>> Check if the tunnel to the target server is working.")
		os.Exit(1)
	}

	fmt.Printf("✓ Received UDP response (%d bytes)\n", n)
	fmt.Println("\n=== UDP ASSOCIATE TEST PASSED ===")
}

// buildDNSQuery builds a simple DNS A record query
func buildDNSQuery(domain string) []byte {
	// Transaction ID (random)
	query := []byte{0x12, 0x34}
	// Flags: standard query
	query = append(query, 0x01, 0x00)
	// QDCOUNT = 1
	query = append(query, 0x00, 0x01)
	// ANCOUNT, NSCOUNT, ARCOUNT = 0
	query = append(query, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)

	// QNAME
	parts := []string{"example", "com"}
	for _, part := range parts {
		query = append(query, byte(len(part)))
		query = append(query, []byte(part)...)
	}
	query = append(query, 0x00) // null terminator

	// QTYPE = A (1)
	query = append(query, 0x00, 0x01)
	// QCLASS = IN (1)
	query = append(query, 0x00, 0x01)

	return query
}
