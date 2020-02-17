package slim_test

// func TestServerStreamNominal(t *testing.T) {
// 	t.Parallel()
// 	// log := log2.NewTest(t, log2.LDebug)
// 	log := log2.NewStderr(log2.LDebug) // useful with panics
// 	log.SetFlags(log2.Lmicroseconds | log2.Lshortfile)
// 	servopt := slim.ServerOptions{
// 		Log: log,
// 		// OnAuth: func(conn slim.Conn, p *tele.Packet) error {
// 		// 	log.Printf("server: auth conn=%s p=%s", conn, p)
// 		// 	return
// 		// },
// 		OnClose: func(authid string, vmid tele.VMID, e error) {
// 			log.Printf("server: id=%s vmid=%d disconnected", authid, vmid)
// 		},
// 		OnPacket: func(conn slim.Conn, p *tele.Packet) error {
// 			return nil
// 		},
// 	}
// 	server, cli := testPairStream(t, &servopt, &slim.ListenOptions{NetworkTimeout: time.Second}, nil)
// 	time.Sleep(time.Second)
// 	cli.Close()
// 	t.Logf("client: stat=%s", cli.Stat().String())
// 	t.Logf("server: stat=%s", server.Stat().String())
// }

// func testServerStream(t testing.TB, servopt *slim.ServerOptions, lopt *slim.ListenOptions) (*slim.Server, string) {
// 	if lopt.StreamURL == "" {
// 		lopt.StreamURL = "tcp://"
// 	}
// 	s := slim.NewServer(*servopt)
// 	require.NoError(t, s.Listen(context.Background(), []slim.ListenOptions{*lopt}))
// 	addrs := s.Addrs()
// 	require.Equal(t, 1, len(addrs))
// 	return s, addrs[0]
// }

// func testPairStream(t testing.TB, servopt *slim.ServerOptions, lopt *slim.ListenOptions, cliopt *slim.ClientOptions) (*slim.Server, *slim.Client) {
// 	s, addr := testServerStream(t, servopt, lopt)
// 	if cliopt == nil {
// 		cliopt = testClientOptions(nil, "")
// 	}
// 	cliopt.Log = servopt.Log
// 	if cliopt.OnPacket == nil {
// 		cliopt.OnPacket = func(conn slim.Conn, p *tele.Packet) error {
// 			from, _ := conn.ID()
// 			cliopt.Log.Printf("client: onpacket from=%s p=%s", from, p)
// 			return nil
// 		}
// 	}
// 	cliopt.StreamURL = "tcp://" + addr // TODO scheme from lopt
// 	cliopt.VmId = tele.VMID(rand.Uint32())
// 	c, err := slim.NewClient(cliopt)
// 	require.NoError(t, err)
// 	return s, c
// }
