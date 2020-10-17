package gomqtt

import (
	"fmt"

	"github.com/256dpi/gomqtt/packet"
)

// Minor readability improvement
// PUBLISH payload as hex, no duplicate "Message=<Message"
// Redundant after https://github.com/256dpi/gomqtt/commit/fc6876a8dbb451a2e1423b40f35ea3d3806660e9
func PacketString(p packet.Generic) string {
	if p == nil {
		return "(nil)"
	}
	if pub, ok := p.(*packet.Publish); ok {
		return fmt.Sprintf("<Publish ID=%d Dup=%t %s>", pub.ID, pub.Dup, MessageString(&pub.Message))
	}
	return p.String()
}

func MessageString(m *packet.Message) string {
	if m == nil {
		return "message=nil"
	}
	return fmt.Sprintf("Topic=%q QOS=%d Retain=%t Payload=%x", m.Topic, m.QOS, m.Retain, m.Payload)
}
