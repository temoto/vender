// Custom binary protocol for vender telemetry.
// Differences from previous implementation using MQTT:
// - separation of transient (State) and guaranteed persistence (Telemetry) messages
// was difficult to achieve with existing MQTT libraries
// - much simpler code
// - smaller network footprint
// - possible (not implemented) to run over UDP
// - possible (not implemented) rudimentary time synchronization
//
// Endpoints exchange "frames" which incapsulate network related details.
// Frame payload is exactly one application level Packet.
// Frames sent over insecure channel are authenticated using HMAC.
package telenet
