package ui_config

type Config struct { //nolint:maligned
	Front struct {
		MsgError        string `hcl:"msg_error"`
		MsgMenuError    string `hcl:"msg_menu_error"`
		MsgStateBroken  string `hcl:"msg_broken"`
		MsgStateLocked  string `hcl:"msg_locked"`
		MsgStateIntro   string `hcl:"msg_intro"`
		MsgWait         string `hcl:"msg_wait"`
		MsgWaterTemp    string `hcl:"msg_water_temp"`
		ResetTimeoutSec int    `hcl:"reset_sec"`
	}

	Service struct {
		Auth struct {
			Enable    bool     `hcl:"enable"`
			Passwords []string `hcl:"passwords"`
		}
		MsgAuth         string `hcl:"msg_auth"`
		ResetTimeoutSec int    `hcl:"reset_sec"`
		Tests           []struct {
			Name     string `hcl:"name,key"`
			Scenario string `hcl:"scenario"`
		} `hcl:"test"`
	}
}
