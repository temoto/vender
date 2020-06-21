package ui_config

type Config struct { //nolint:maligned
	Front struct {
		MsgError       string `hcl:"msg_error"`
		MsgMenuError   string `hcl:"msg_menu_error"`
		MsgStateBroken string `hcl:"msg_broken"`
		MsgStateLocked string `hcl:"msg_locked"`
		MsgStateIntro  string `hcl:"msg_intro"`
		MsgWait        string `hcl:"msg_wait"`
		MsgWaterTemp   string `hcl:"msg_water_temp"`

		MsgMenuCodeEmpty          string `hcl:"msg_menu_code_empty"`
		MsgMenuCodeInvalid        string `hcl:"msg_menu_code_invalid"`
		MsgMenuInsufficientCredit string `hcl:"msg_menu_insufficient_credit"`
		MsgMenuNotAvailable       string `hcl:"msg_menu_not_available"`

		MsgCream   string `hcl:"msg_cream"`
		MsgSugar   string `hcl:"msg_sugar"`
		MsgCredit  string `hcl:"msg_credit"`
		MsgMaking1 string `hcl:"msg_making1"`
		MsgMaking2 string `hcl:"msg_making2"`

		MsgInputCode string `hcl:"msg_input_code"`

		ResetTimeoutSec int `hcl:"reset_sec"`
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
