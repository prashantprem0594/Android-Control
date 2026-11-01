package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeIPv6LoopbackNonceEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var completed atomic.Int64
	done := make(chan struct{})
	go func() { defer close(done); serveTCP(listener, &completed) }()
	connection, err := net.DialTimeout("tcp6", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if connection.RemoteAddr().(*net.TCPAddr).IP.To4() != nil {
		t.Fatal("IPv4-mapped endpoint used as IPv6")
	}
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	const nonce = "0123456789abcdef0123456789abcdef"
	if _, err := io.WriteString(connection, "APP_CONTROL_NET_FIXTURE_V1:"+nonce+"\n"); err != nil {
		t.Fatal(err)
	}
	response, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil || response != "APP_CONTROL_NET_OK_V1:"+nonce+"\n" {
		t.Fatal("native IPv6 nonce exchange failed")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("IPv6 fixture failed to stop")
	}
	if completed.Load() != 1 {
		t.Fatal("IPv6 handshake missing")
	}
}

func dnsQuestion() []byte {
	query := make([]byte, 12)
	binary.BigEndian.PutUint16(query[0:2], 0x1234)
	binary.BigEndian.PutUint16(query[2:4], 0x0100)
	binary.BigEndian.PutUint16(query[4:6], 1)
	return append(query, []byte("\x200123456789abcdef0123456789abcdef\x07fixture\x07invalid\x00\x00\x01\x00\x01")...)
}

func TestDNSWireEndpointRoundTripAndShutdown(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	var completed atomic.Int64
	done := make(chan struct{})
	go func() { defer close(done); serveDNS(server, &completed) }()
	client, err := net.Dial("udp4", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	query := dnsQuestion()
	if _, err := client.Write(query); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 513)
	size, err := client.Read(buffer)
	if err != nil || !bytes.Equal(buffer[:size], dnsResponse(query)) {
		t.Fatal("local DNS-wire exchange failed")
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DNS fixture failed to stop")
	}
	if completed.Load() != 1 {
		t.Fatal("DNS exchange was not counted")
	}
}

func TestDNSWireFixtureAnswersOnlyExactFreshFixtureQuestion(t *testing.T) {
	query := dnsQuestion()
	response := dnsResponse(query)
	if len(response) != 82 || !bytes.Equal(response[:2], query[:2]) ||
		binary.BigEndian.Uint16(response[2:4]) != 0x8180 ||
		binary.BigEndian.Uint16(response[6:8]) != 1 || !bytes.Equal(response[12:66], query[12:]) ||
		!bytes.Equal(response[66:], []byte{0xc0, 12, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 192, 0, 2, 123}) {
		t.Fatal("DNS response lost transaction/question binding or reserved answer")
	}
	for _, index := range []int{2, 4, 6, 8, 10, 12, 13, 45, 46, 53, 54, 61, 63, 65} {
		invalid := append([]byte(nil), query...)
		invalid[index] ^= 0x80
		if dnsResponse(invalid) != nil {
			t.Fatalf("malformed or out-of-scope question accepted at byte %d", index)
		}
	}
	for _, invalid := range [][]byte{nil, query[:65], append(query, 0), make([]byte, 513)} {
		if dnsResponse(invalid) != nil {
			t.Fatal("unbounded or incomplete DNS question accepted")
		}
	}
}
