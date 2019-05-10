package util

//  HTTP reseponse time data structure and methods

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// Components of an HTTP ping request for reporting performance (to cloudwatch, or whatever)
type PingTimes struct {
	Start    time.Time     // time we started the ping
	DnsLk    time.Duration // DNS Lookup
	TcpHs    time.Duration // TCP Handshake
	TlsHs    time.Duration // TLS Handshake
	Reply    time.Duration // HTTP Reply (first byte)
	Close    time.Duration // HTTP Reply (last byte / closed)
	Total    time.Duration // (Calculated) Total response time (see RespTime() below)
	DestUrl  *string       // URL that received the request
	Location *string       // Client location, City,Country
	Remote   string        // Server IP from DNS resolution
	RespCode int           // HTTP response code or -1 (for network failure)
	Size     int64         // total response bytes
}

// Response time is the total duration from the TCP open until the TCP close.
// DNS lookup time is not included in this measure.
// Will be zero iff the request failed.
// This method sets the value in the object to the sum.  Call this before dumping as JSON!
func (pt *PingTimes) RespTime() time.Duration {
	if pt.Total == 0 {
		pt.Total = pt.DnsLk + pt.TcpHs + pt.TlsHs + pt.Reply + pt.Close
	}
	return pt.Total
}

// Msec returns the duration as a floating point number of seconds.
func Msec(d time.Duration) float64 {
	sec := d / time.Second
	nsec := d % time.Second
	return float64(sec*1e3) + float64(nsec)/1e6
}

var myIp *string

// Return my outbound IP address as a string
func GetMyIp() string {
	if myIp == nil {
		conn, err := net.Dial("udp", "8.8.8.8:53")
		if err != nil {
			return "getIpErr"
		}
		defer conn.Close()
		myAddr := HostNoPort(conn.LocalAddr().String())
		myIp = &myAddr
	}
	return *myIp
}

func LocationOrIp(loc *string) string {
	if loc != nil && len(*loc) > 0 {
		return *loc
	} else {
		return GetMyIp()
	}
}

func (pt *PingTimes) String() string {
	return fmt.Sprintln(
		"DnsLk:", pt.DnsLk, // DNS lookup
		"TcpHs:", pt.TcpHs, // tcp connection
		"TlsHs:", pt.TlsHs, // TLS handshake
		"Reply:", pt.Reply, // server processing: first byte time
		"Close:", pt.Close, // time to last byte
		"Remote:", pt.Remote, // Server IP from DNS resolution
		"Resp:", pt.RespCode,
		"Size:", pt.Size,
	)
}

func SafeStrPtr(sp *string, ifnil string) string {
	if sp == nil || *sp == "" {
		return ifnil
	}
	return *sp
}

// Return tab separated values: Unix timestamp first then msec time values for
// each of the time component fields as msec.uuu (three digits of microseconds).
func (pt *PingTimes) MsecTsv() string {
	return fmt.Sprintf("%d\t%.03f\t%.03f\t%.03f\t%.03f\t%.03f\t%.03f\t%03d\t%d\t%s\t%s\t%s",
		pt.Start.Unix(),
		Msec(pt.DnsLk),
		Msec(pt.TcpHs),
		Msec(pt.TlsHs),
		Msec(pt.Reply),
		Msec(pt.Close),
		Msec(pt.RespTime()),
		pt.RespCode,
		pt.Size,
		LocationOrIp(pt.Location),
		pt.Remote,
		SafeStrPtr(pt.DestUrl, "noUrl"))
}

func TextHeader(file *os.File) {
	fmt.Fprintf(file, "# %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		"timestamp",
		"DNS",
		"TCP",
		"TLS",
		"First",
		"LastB",
		"Total",
		"HTTP",
		"Size",
		"From_Location",
		"Remote_Addr",
		"proto://uri")
}

// Write ping times as tab-separated milliseconds into the given open file.
func (pt *PingTimes) DumpText(file *os.File) {
	fmt.Fprintln(file, pt.MsecTsv())
}

// Write ping times as raw JSON (nanosecond values) into the given open file.
func (pt *PingTimes) DumpJson(file *os.File) error {
	enc := json.NewEncoder(file)
	enc.SetIndent("", " ")
	enc.Encode(pt)
	return nil
}
