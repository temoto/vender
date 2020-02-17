package mqtt

import (
	"net"
	"strings"
	"time"
)

func addrString(a net.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

func defaultString(main, def string) string {
	if main == "" {
		return def
	}
	return main
}

func isClosedConn(e error) bool {
	return e != nil && strings.HasSuffix(e.Error(), "use of closed network connection")
}

func keepaliveAndHalf(sec uint16) time.Duration {
	d := time.Duration(sec) * time.Second
	return d + d/2
}
