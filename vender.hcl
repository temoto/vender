mdb {
  debug            = true
  uart_driver      = "file"
  uart_device_path = "/dev/ttyAMA0"
}

papa {
  address = "127.0.0.1:50051"
}
