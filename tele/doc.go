// Package tele is vender telemetry network/binary API for external usage.
// Current implementation is MQTT protocol with protobuf payload from tele.proto.
// Teler.State() sends 1 byte(enum State) to topic vm%d/w/1s
package tele
