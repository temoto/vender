// Package slim provides reliable secure message transport with small network footprint.
//
// Features:
// - client-server connection topology
// - bidirectional message delivery, request-response is not imposed
// - aims to smallest network overhead
// - application controls handshake
// - application assigns custom ID to each connection
// - optional HMAC signature
// - ack sent after application processing
// - statistic counters
//
// Out of scope:
// - large messages. 64k should be enough for everyone.
// - ordered delivery. Check frame sequence.
// - fragmentation left to underlying protocol.
//
// Kind of alternative to MQTT wire protocol.
//
// Inspired by:
// - SCTP
// - QUIC
// - https://gafferongames.com/post/reliable_ordered_messages/
package slim
