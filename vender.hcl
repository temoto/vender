mdb {
  log_enable  = false
  uart_driver = "file"
  uart_device = "/dev/ttyAMA0"
}

papa {
  address = "127.0.0.1:50051"
}
