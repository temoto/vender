// Separate package to for hardware/evend related config structure.
// Ulgy workaround to import cycles.
package tele_config

type Config struct { //nolint:maligned
	ConnectTimeoutSec int    `hcl:"connect_timeout_sec"`
	Enabled           bool   `hcl:"enable"`
	VmId              int    `hcl:"vm_id"`
	LogDebug          bool   `hcl:"log_debug"`
	MqttBroker        string `hcl:"mqtt_broker"`
	MqttKeepaliveSec  int    `hcl:"keepalive_sec"`
	MqttLogDebug      bool   `hcl:"mqtt_log_debug"`
	MqttPassword      string `hcl:"mqtt_password"` // secret
	Persist           string `hcl:"mqtt_store_path"`
	StateIntervalSec  int    `hcl:"state_interval_sec"`
	TlsCaFile         string `hcl:"tls_ca_file"`
	TlsPsk            string `hcl:"tls_psk"` // secret
}
