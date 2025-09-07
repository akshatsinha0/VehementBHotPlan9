package core

import (
	"bufio"
	"net"
	"strings"
	"sync"
)

type MetricsClient struct {
	mu sync.Mutex
	c  net.Conn
	br *bufio.Reader
}

func NewMetricsClient(addr string) (*MetricsClient, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil { return nil, err }
	return &MetricsClient{c: c, br: bufio.NewReader(c)}, nil
}

func (m *MetricsClient) send(cmd string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.c.Write([]byte(cmd+"\n"))
	if err != nil { return "", err }
	var sb strings.Builder
	for {
		line, err := m.br.ReadString('\n')
		if err != nil { return sb.String(), err }
		if strings.HasPrefix(line, ".\n") { break }
		sb.WriteString(line)
	}
	return sb.String(), nil
}

func (m *MetricsClient) IncCounter(k string, v int64) (string, error) { return m.send("inc "+k+" "+itoa(v)) }
func (m *MetricsClient) SetGauge(k string, v int64) (string, error) { return m.send("set "+k+" "+itoa(v)) }
func (m *MetricsClient) ResetCounter(k string) (string, error) { return m.send("rc "+k) }
func (m *MetricsClient) ResetGauge(k string) (string, error) { return m.send("rg "+k) }
func (m *MetricsClient) DumpCounters() (string, error) { return m.send("dc") }
func (m *MetricsClient) DumpGauges() (string, error) { return m.send("dg") }

func itoa(v int64) string {
	s := ""
	n := v
	if n == 0 { return "0" }
	neg := n < 0
	if neg { n = -n }
	for n > 0 {
		d := n % 10
		s = string('0'+d) + s
		n /= 10
	}
	if neg { s = "-" + s }
	return s
}