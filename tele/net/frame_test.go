package telenet_test

// func TestFrameEncode(t *testing.T) {
// 	t.Parallel()
// 	rnd := helpers.RandUnix()
// 	gen := func(r *rand.Rand) *tele.Frame {
// 		p := &tele.Packet{Time: r.Int63()}
// 		f := telenet.NewFrame(uint16(r.Uint32()), p)
// 		f.Sack = r.Uint32()
// 		f.Acks = r.Uint32()
// 		return f
// 	}
// 	_ = rnd
// 	_ = gen
// }
