// Local emulator acceptance endpoint. It never proxies, authenticates or records traffic.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Only randomized fixture.invalid A questions are answered. This is a local
// DNS-wire packet endpoint, never a recursive resolver or arbitrary proxy.
func dnsResponse(query []byte) []byte {
	if len(query) != 66 || binary.BigEndian.Uint16(query[2:4]) != 0x0100 ||
		binary.BigEndian.Uint16(query[4:6]) != 1 || !bytes.Equal(query[6:12], make([]byte, 6)) ||
		query[12] != 32 || !bytes.Equal(query[45:], []byte("\x07fixture\x07invalid\x00\x00\x01\x00\x01")) {
		return nil
	}
	nonce := string(query[13:45])
	decoded, err := hex.DecodeString(nonce)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != nonce {
		return nil
	}
	response := append([]byte(nil), query...)
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[6:8], 1)
	return append(response, []byte{0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 192, 0, 2, 123}...)
}

func serveDNS(connection net.PacketConn, completed *atomic.Int64) {
	buffer := make([]byte, 513)
	for {
		size, peer, err := connection.ReadFrom(buffer)
		if err != nil {
			return
		}
		if response := dnsResponse(buffer[:size]); response != nil {
			if _, err := connection.WriteTo(response, peer); err == nil {
				completed.Add(1)
			}
		}
	}
}

func serve(connection net.Conn, completed *atomic.Int64) {
	defer connection.Close()
	if connection.SetDeadline(time.Now().Add(2*time.Second)) != nil {
		return
	}
	line, err := bufio.NewReader(io.LimitReader(connection, 97)).ReadString('\n')
	const prefix = "APP_CONTROL_NET_FIXTURE_V1:"
	if err != nil || len(line) > 96 || !strings.HasPrefix(line, prefix) {
		return
	}
	nonce := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "\n")
	decoded, err := hex.DecodeString(nonce)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != nonce {
		return
	}
	if _, err := io.WriteString(connection, "APP_CONTROL_NET_OK_V1:"+nonce+"\n"); err == nil {
		completed.Add(1)
	}
}

func serveTCP(listener net.Listener, completed *atomic.Int64) {
	var workers sync.WaitGroup
	defer workers.Wait()
	capacity := make(chan struct{}, 8)
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		select {
		case capacity <- struct{}{}:
			workers.Add(1)
			go func() { defer workers.Done(); defer func() { <-capacity }(); serve(connection, completed) }()
		default:
			_ = connection.Close()
		}
	}
}

func run() error {
	address := flag.String("listen", "127.0.0.1:19089", "loopback-only acceptance endpoint")
	dnsAddress := flag.String("dns-listen", "127.0.0.1:19090", "loopback-only DNS-wire acceptance endpoint")
	ipv6Address := flag.String("ipv6-listen", "[::1]:19091", "native IPv6 loopback-only nonce endpoint")
	duration := flag.Duration("duration", 15*time.Minute, "bounded lifetime (30s through 30m)")
	ready := flag.String("ready-file", "", "new public readiness file")
	flag.Parse()
	host, port, err := net.SplitHostPort(*address)
	number, conversion := strconv.Atoi(port)
	if err != nil || conversion != nil || host != "127.0.0.1" || number < 1024 || number > 65535 || flag.NArg() != 0 || *duration < 30*time.Second || *duration > 30*time.Minute || *ready == "" {
		return fmt.Errorf("invalid fixture options")
	}
	dnsHost, dnsPort, err := net.SplitHostPort(*dnsAddress)
	dnsNumber, dnsConversion := strconv.Atoi(dnsPort)
	if err != nil || dnsConversion != nil || dnsHost != "127.0.0.1" || dnsNumber < 1024 || dnsNumber > 65535 {
		return fmt.Errorf("invalid fixture options")
	}
	ipv6Host, ipv6Port, err := net.SplitHostPort(*ipv6Address)
	ipv6Number, ipv6Conversion := strconv.Atoi(ipv6Port)
	if err != nil || ipv6Conversion != nil || ipv6Host != "::1" || ipv6Number < 1024 || ipv6Number > 65535 {
		return fmt.Errorf("invalid fixture options")
	}
	listener, err := net.Listen("tcp4", *address)
	if err != nil {
		return err
	}
	defer listener.Close()
	dns, err := net.ListenPacket("udp4", *dnsAddress)
	if err != nil {
		return err
	}
	defer dns.Close()
	ipv6, err := net.Listen("tcp6", *ipv6Address)
	if err != nil {
		return err
	}
	defer ipv6.Close()
	file, err := os.OpenFile(*ready, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(file, "network-fixture-v3 %s dns=%s ipv6=%s\n", listener.Addr(), dns.LocalAddr(), ipv6.Addr())
	syncErr, closeErr := file.Sync(), file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("readiness failed")
	}
	fmt.Println("Loopback network acceptance fixture ready; payloads and peers are never logged.")
	timer := time.AfterFunc(*duration, func() { _ = listener.Close(); _ = dns.Close(); _ = ipv6.Close() })
	defer timer.Stop()
	var completed atomic.Int64
	var workers sync.WaitGroup
	workers.Add(1)
	go func() { defer workers.Done(); serveDNS(dns, &completed) }()
	workers.Add(1)
	go func() { defer workers.Done(); serveTCP(ipv6, &completed) }()
	capacity := make(chan struct{}, 8)
	for {
		connection, err := listener.Accept()
		if err != nil {
			break
		}
		select {
		case capacity <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-capacity }()
				serve(connection, &completed)
			}()
		default:
			_ = connection.Close()
		}
	}
	_ = dns.Close()
	_ = ipv6.Close()
	workers.Wait()
	fmt.Printf("Network fixture stopped; completed handshakes=%d\n", completed.Load())
	return nil
}

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "Local network fixture failed; check public options, endpoint and readiness path.")
		os.Exit(1)
	}
}
