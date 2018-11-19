iodin_path = "target/release/iodin"

mdb {
  log_enable = false

  uart_driver = "file"
  uart_device = "/dev/ttyAMA0"

  #uart_driver = "iodin"
  #uart_device = "\x0f\x0e"
}

papa {
  address = "127.0.0.1:50051"
  enabled = false
}
