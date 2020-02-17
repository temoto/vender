package tele

func NewPacketHello(timestamp int64, authid string, vmid VMID) *Packet {
	return &Packet{
		AuthId: authid,
		VmId:   int32(vmid),
		Time:   timestamp,
		Hello:  true,
	}
}
