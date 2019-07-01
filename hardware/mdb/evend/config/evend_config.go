// Separate package to for hardware/evend related config structure.
// Ugly workaround to import cycles.
package evend_config

type Config struct { //nolint:maligned
	Conveyor struct { //nolint:maligned
		MinSpeed    int `hcl:"min_speed"`
		PositionMax int `hcl:"position_max"`
	} `hcl:"conveyor"`
	Cup struct { //nolint:maligned
		AssertBusyDelayMs  int `hcl:"assert_busy_delay_ms"`
		DispenseTimeoutSec int `hcl:"dispense_timeout_sec"`
		EnsureTimeoutSec   int `hcl:"ensure_timeout_sec"`
	} `hcl:"cup"`
	Elevator struct { //nolint:maligned
		TimeoutSec int `hcl:"timeout_sec"`
	} `hcl:"elevator"`
	Espresso struct { //nolint:maligned
		StockRate  float32 `hcl:"stock_rate"`
		TimeoutSec int     `hcl:"timeout_sec"`
	} `hcl:"espresso"`
	Hopper struct { //nolint:maligned
		RunTimeoutMs     int     `hcl:"run_timeout_ms"`
		DefaultStockRate float32 `hcl:"default_stock_rate"`
	} `hcl:"hopper"`
	Mixer struct { //nolint:maligned
		MoveTimeoutSec int `hcl:"move_timeout_sec"`
		ShakeTimeoutMs int `hcl:"shake_timeout_ms"`
	} `hcl:"mixer"`
	Valve struct { //nolint:maligned
		// TODO TemperatureCold int     `hcl:"temperature_cold"`
		TemperatureHot int     `hcl:"temperature_hot"`
		PourTimeoutSec int     `hcl:"pour_timeout_sec"`
		WaterStockRate float32 `hcl:"water_stock_rate"`
		CautionPartMl  int     `hcl:"caution_part_ml"`
	} `hcl:"valve"`
}
