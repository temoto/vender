engine {
  // alias "cup_dispense" { scenario = "conveyor_move_cup cup_drop" }

  menu {
    msg_intro = "TODO_EDIT showed after successful boot"
    reset_sec = 180

    item "1" {
      name     = "example"
      price    = 1
      scenario = "cup_dispense water_hot(10) put_milk(10) cup_serve"
    }
  }

  // on_start = ["mixer_move_top", "cup_serve", "conveyor_move_cup"]
}

hardware {
  hd44780 {
    codepage = "windows-1251"
    enable   = true

    pinmap {
      rs = "23"
      rw = "18"
      e  = "24"
      d4 = "22"
      d5 = "21"
      d6 = "17"
      d7 = "7"
    }

    blink        = true
    cursor       = false
    scroll_delay = 210
    width        = 16
  }

  input {
    evend_keyboard {
      enable = true

      // TODO listen_addr = 0x78
    }

    dev_input_event {
      enable = true
      device = "/dev/input/event0"
    }
  }

  iodin_path = "TODO_EDIT"

  mega {
    spi      = ""
    pin_chip = "/dev/gpiochip0"
    pin      = "25"
  }

  mdb {
    // log_debug = true
    log_debug = false

    uart_driver = "mega"

    #uart_driver = "file"
    #uart_device = "/dev/ttyAMA0"

    #uart_driver = "iodin"
    #uart_device = "\x0f\x0e"
  }
}

money {
  // Multiple of lowest money unit for config convenience and formatting.
  // All money numbers in config are multipled by scale.
  // For USD/EUR set `scale=1` and specify prices in cents.
  scale = 100

  credit_max = 200

  // limit to over-compensate change return when exact amount is not available
  change_over_compensate = 10
}

tele {
  enable          = false
  vm_id           = -1
  log_debug       = true
  mqtt_log_debug  = false
  mqtt_broker     = "tls://TODO_EDIT:8884"
  mqtt_password   = "TODO_EDIT"
  mqtt_store_path = ""
  tls_ca_file     = "TODO_EDIT"
}

include "local.hcl" {
  optional = true
}
