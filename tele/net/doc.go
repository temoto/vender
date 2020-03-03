// Custom binary protocol for vender telemetry.
// Differences from previous implementation using MQTT:
// - separation of transient (State) and guaranteed persistence (Telemetry) messages
//   was difficult to achieve with existing MQTT libraries
// - much simpler code
// - smaller network footprint
// - possible (not implemented) to run over UDP
// - possible (not implemented) rudimentary time synchronization
package telenet
