hardware {
  iodin_path = "target/release/iodin"

  // TODO keyboard_listen_addr = 0x78

  mega {
    spi = ""
    pin = "25"
  }
  mdb {
    log_enable = true

    uart_driver = "mega"

    #uart_driver = "file"
    #uart_device = "/dev/ttyAMA0"

    #uart_driver = "iodin"
    #uart_device = "\x0f\x0e"
  }
}

papa {
  address = "127.0.0.1:50051"
  enabled = false
}
