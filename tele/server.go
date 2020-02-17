package tele

// if f.Packet == nil {
// 	err = tele.ErrUnexpectedPacket
// 	return errors.Trace(err)
// }
// if !f.Packet.Hello {
// 	err = tele.ErrUnexpectedPacket
// 	return errors.Trace(err)
// }
// hello := f.Packet
// if hello.AuthId == "" {
// 	hello.AuthId = fmt.Sprintf("vm%d", hello.VmId)
// }

// response := &Frame{
// Hello: true,
// Time:  time.Now().UnixNano(),
// TODO SessionId:  s.newsid(),
// }
// if err = s.authorize(conn, hello); err != nil {
// 	return errors.Annotate(err, "authorize")
// }

// func (s *Server) authorize(conn Conn, f*Frame) error {
// 	wantRole := tele_config.RoleInvalid
// 	switch {
// 	case p.Hello && p.AuthId == fmt.Sprintf("vm%d", p.VmId):
// 		wantRole = tele_config.RoleVender

// 	case p.Hello:
// 		wantRole = tele_config.RoleAll

// 	case p.State != tele.State_Invalid || p.Telemetry != nil:
// 		wantRole = tele_config.RoleVender

// 	case p.Command != nil || p.Response != nil:
// 		wantRole = tele_config.RoleControl
// 	}

// 	if wantRole == tele_config.RoleInvalid {
// 		return fmt.Errorf("code error authorize cant infer required role")
// 	}
// 	// TODO
// 	return nil
// }
