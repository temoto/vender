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

  keyboard {
    enable = true
  }

  iodin_path = "target/release/iodin"

  // TODO keyboard_listen_addr = 0x78

  mega {
    spi = ""
    pin = "25"
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

papa {
  address = "127.0.0.1:50051"
  enabled = false
}
